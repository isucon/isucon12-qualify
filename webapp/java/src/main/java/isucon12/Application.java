package isucon12;

import java.io.BufferedReader;
import java.io.File;
import java.io.IOException;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.security.KeyFactory;
import java.security.NoSuchAlgorithmException;
import java.security.interfaces.RSAPublicKey;
import java.security.spec.InvalidKeySpecException;
import java.security.spec.X509EncodedKeySpec;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Base64;
import java.util.Collections;
import java.util.Comparator;
import java.util.Date;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import java.util.stream.Collectors;
import java.util.stream.Stream;

import javax.servlet.http.Cookie;
import javax.servlet.http.HttpServletRequest;

import org.apache.commons.lang3.StringUtils;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.dao.DataAccessException;
import org.springframework.dao.EmptyResultDataAccessException;
import org.springframework.http.HttpStatus;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.jdbc.core.namedparam.SqlParameterSource;
import org.springframework.jdbc.support.GeneratedKeyHolder;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.ModelAttribute;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.multipart.MultipartFile;

import com.auth0.jwt.JWT;
import com.auth0.jwt.JWTVerifier;
import com.auth0.jwt.algorithms.Algorithm;
import com.auth0.jwt.exceptions.JWTVerificationException;
import com.auth0.jwt.interfaces.DecodedJWT;

import isucon12.exception.AuthorizePlayerException;
import isucon12.exception.BillingReportByCompetitionException;
import isucon12.exception.DatabaseException;
import isucon12.exception.DispenseIdException;
import isucon12.exception.RetrieveCompetitionException;
import isucon12.exception.RetrievePlayerException;
import isucon12.exception.RetrieveTenantRowFromHeaderException;
import isucon12.exception.WebException;
import isucon12.json.BillingHandlerResult;
import isucon12.json.BillingReport;
import isucon12.json.CompetitionDetail;
import isucon12.json.CompetitionRank;
import isucon12.json.CompetitionRankingHandlerResult;
import isucon12.json.CompetitionsAddHandlerResult;
import isucon12.json.CompetitionsHandlerResult;
import isucon12.json.InitializeHandlerResult;
import isucon12.json.MeHandlerResult;
import isucon12.json.PlayerDetail;
import isucon12.json.PlayerDisqualifiedHandlerResult;
import isucon12.json.PlayerHandlerResult;
import isucon12.json.PlayerScoreDetail;
import isucon12.json.PlayersAddHandlerResult;
import isucon12.json.PlayersListHandlerResult;
import isucon12.json.ScoreHandlerResult;
import isucon12.json.SuccessResult;
import isucon12.json.TenantDetail;
import isucon12.json.TenantsAddHandlerResult;
import isucon12.json.TenantsBillingHandlerResult;
import isucon12.model.CompetitionRow;
import isucon12.model.PlayerRow;
import isucon12.model.PlayerScoreRow;
import isucon12.model.TenantRow;
import isucon12.model.TenantWithBilling;
import isucon12.model.Viewer;
import isucon12.model.VisitHistorySummaryRow;

@SpringBootApplication
@RestController
public class Application {
    @Autowired
    private NamedParameterJdbcTemplate adminDb;

    Logger logger = LoggerFactory.getLogger(Application.class);

    private static final String TENANT_DB_SCHEMA_FILE_PATH = "../sql/tenant/10_schema.sql";
    private static final String INITIALIZE_SCRIPT = "../sql/init.sh";
    private static final String COOKIE_NAME = "isuports_session";

    private static final String ROLE_ADMIN = "admin";
    private static final String ROLE_ORGANIZER = "organizer";
    private static final String ROLE_PLAYER = "player";
    private static final String ROLE_NONE = "none";

    private static final String TENANT_NAME_REG_PATTERN = "^[a-z][a-z0-9-]{0,61}[a-z0-9]$";
    private static final int SQLITE_BUSY_TIMEOUT = 5;

    /*
     * ENV
     *
     * @Value("${<<環境変数>>:<<デフォルト値>>}")
     */
    @Value("${ISUCON_TENANT_DB_DIR:../tenant_db}")
    private String ISUCON_TENANT_DB_DIR;
    @Value("${SERVER_APP_PORT:3000}")
    private Integer SERVER_APP_PORT;
    @Value("${ISUCON_JWT_KEY_FILE:./public.pem}")
    private String ISUCON_JWT_KEY_FILE;
    @Value("${ISUCON_BASE_HOSTNAME:.t.isucon.dev}")
    private String ISUCON_BASE_HOSTNAME;
    @Value("${ISUCON_ADMIN_HOSTNAME:admin.t.isucon.dev}")
    private String ISUCON_ADMIN_HOSTNAME;

    public String tenantDBPath(long id) {
        return Paths.get(ISUCON_TENANT_DB_DIR).resolve(String.format("%d.db", id)).toString();
    }

    public Connection connectToTenantDB(long id) throws DatabaseException {
        String tenantDBPath = this.tenantDBPath(id);
        if (!new File(tenantDBPath).exists()) {
            throw new DatabaseException(String.format("failed to open tenant DB: %s", tenantDBPath));
        }

        try {
            return DriverManager.getConnection(String.format("jdbc:log4jdbc:sqlite:%s", tenantDBPath));
        } catch (SQLException e) {
            throw new DatabaseException(String.format("failed to open tenant DB: %s", e.getMessage()));
        }
    }

    public void createTenantDB(long id) {
        String tenantDBPath = this.tenantDBPath(id);

        try {
            Process p = new ProcessBuilder().command("sh", "-c", String.format("sqlite3 %s < %s", tenantDBPath, TENANT_DB_SCHEMA_FILE_PATH)).start();
            int exitCode = p.waitFor();
            p.destroy();
            if (exitCode != 0) {
                InputStreamReader inputStreamReader = new InputStreamReader(p.getErrorStream());
                Stream<String> streamOfString = new BufferedReader(inputStreamReader).lines();
                String message = streamOfString.collect(Collectors.joining());

                throw new RuntimeException(String.format("failed to exec sqlite3 %s < %s, out=%s", tenantDBPath, TENANT_DB_SCHEMA_FILE_PATH, message));
            }
        } catch (IOException | InterruptedException e) {
            throw new RuntimeException(String.format("failed to exec sqlite3 %s < %s, %s", tenantDBPath, TENANT_DB_SCHEMA_FILE_PATH, e));
        }
    }

    // システム全体で一意なIDを生成する
    public String dispenseID() throws DispenseIdException {
        String lastErrorString = "";
        GeneratedKeyHolder holder = new GeneratedKeyHolder();
        SqlParameterSource source = new MapSqlParameterSource().addValue("stub", "a");

        for (int i = 0; i < 100; i++) {
            try {
                this.adminDb.update("REPLACE INTO id_generator (stub) VALUES (:stub);", source, holder);
            } catch (DataAccessException e) {
                if (e.getRootCause() instanceof SQLException) {
                    SQLException se = (SQLException) e.getRootCause();
                    // deadlock
                    if (se.getErrorCode() == 1213) {
                        lastErrorString = String.format("error REPLACE INTO id_generator: %s", se.getMessage());
                        continue;
                    }
                }
                throw new DispenseIdException(String.format("error REPLACE INTO id_generator: %s", e.getMessage()));
            }
            if (holder.getKey() == null) {
                throw new DispenseIdException("error get last insert id");
            }
            break;
        }

        if (holder.getKey().longValue() != 0) {
            return String.valueOf(holder.getKey().longValue());
        }
        throw new DispenseIdException(lastErrorString);
    }

    public static void main(String[] args) {
        SpringApplication.run(Application.class, args);
    }

    // リクエストヘッダをパースしてViewerを返す
    private Viewer parseViewer(HttpServletRequest req) {
        if (req.getCookies() == null) {
            throw new WebException(HttpStatus.UNAUTHORIZED, "cookie is null");
        }

        Cookie cookie = Stream.of(req.getCookies())
                .filter(c -> c.getName().equals(COOKIE_NAME))
                .findFirst()
                .orElseThrow(() -> new WebException(HttpStatus.UNAUTHORIZED, String.format("cookie %s is not found", COOKIE_NAME)));

        String token = cookie.getValue();

        DecodedJWT decodedJwt = this.verifyJwt(token, ISUCON_JWT_KEY_FILE);

        if (StringUtils.isEmpty(decodedJwt.getSubject())) {
            throw new WebException(HttpStatus.UNAUTHORIZED, String.format("invalid token: subject is not found in token: %s", token));
        }

        String role = decodedJwt.getClaim("role").asString();
        if (StringUtils.isEmpty(role)) {
            throw new WebException(HttpStatus.UNAUTHORIZED, String.format("invalid token: role is not found in token: %s", token));
        }

        switch (role) {
        case ROLE_ADMIN:
        case ROLE_ORGANIZER:
        case ROLE_PLAYER:
            break;
        default:
            throw new WebException(HttpStatus.UNAUTHORIZED, String.format("invalid token: %s is invalid role: %s", role, token));
        }

        List<String> audiences = decodedJwt.getAudience();
        // audiences は1要素でテナント名がはいっている
        if (audiences.size() != 1) {
            throw new WebException(HttpStatus.UNAUTHORIZED, String.format("invalid token: aud field is few or too much: %s", token));
        }

        TenantRow tenant;
        try {
            tenant = retrieveTenantRowFromHeader(req);
        } catch (RetrieveTenantRowFromHeaderException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error retrieveTenantRowFromHeader at parseViewer: ", e);
        }
        if (tenant == null) {
            throw new WebException(HttpStatus.UNAUTHORIZED, "tenant not found");
        }

        if (tenant.getName().equals("admin") && !role.equals(ROLE_ADMIN)) {
            throw new WebException(HttpStatus.UNAUTHORIZED, "tenant not found");
        }

        if (!tenant.getName().equals(audiences.get(0))) {
            throw new WebException(HttpStatus.UNAUTHORIZED, String.format("invalid token: tenant name is not match with %s: %s", this.getHost(req), token));
        }

        return new Viewer(role, decodedJwt.getSubject(), tenant.getName(), tenant.getId());
    }

    private RSAPublicKey readPublicKeyFromFile(String filePath) {
        try {
            String pem = Files.readAllLines(Paths.get(filePath)).stream()
                    .filter(r -> !r.startsWith("-----BEGIN PUBLIC KEY-----"))
                    .filter(r -> !r.startsWith("-----END PUBLIC KEY-----"))
                    .collect(Collectors.joining());
            X509EncodedKeySpec spec = new X509EncodedKeySpec(Base64.getDecoder().decode(pem));
            return (RSAPublicKey) KeyFactory.getInstance("RSA").generatePublic(spec);
        } catch (IOException e) {
            throw new RuntimeException(String.format("error Files.readAllBytes: keyFilename=%s: ", filePath), e);
        } catch (InvalidKeySpecException | NoSuchAlgorithmException e) {
            throw new RuntimeException("error jwt decode pem:", e);
        }
    }

    private DecodedJWT verifyJwt(String token, String publicKeyFilePath) {
        JWTVerifier jwtVerifier = JWT.require(Algorithm.RSA256(this.readPublicKeyFromFile(publicKeyFilePath), null)).build();

        try {
            return jwtVerifier.verify(token);
        } catch (JWTVerificationException e) {
            throw new WebException(HttpStatus.UNAUTHORIZED, e);
        } catch (Exception e) {
            throw new RuntimeException("fail to parse token: ", e);
        }
    }

    private TenantRow retrieveTenantRowFromHeader(HttpServletRequest req) throws RetrieveTenantRowFromHeaderException {
        // JWTに入っているテナント名とHostヘッダのテナント名が一致しているか確認
        String baseHost = ISUCON_BASE_HOSTNAME;
        String tenantName = StringUtils.removeEnd(this.getHost(req), baseHost);

        // SaaS管理者用ドメイン
        if (tenantName.equals("admin")) {
            return new TenantRow("admin", "admin");
        }

        // テナントの存在確認
        SqlParameterSource source = new MapSqlParameterSource().addValue("name", tenantName);
        RowMapper<TenantRow> mapper = (rs, i) -> {
            TenantRow row = new TenantRow();
            row.setId(rs.getLong("id"));
            row.setName(rs.getString("name"));
            row.setDisplayName(rs.getString("display_name"));
            row.setCreatedAt(new Date(rs.getLong("created_at")));
            row.setUpdatedAt(new Date(rs.getLong("updated_at")));
            return row;
        };

        try {
            return adminDb.queryForObject("SELECT * FROM tenant WHERE name = :name", source, mapper);
        } catch (EmptyResultDataAccessException e) {
            return null;
        } catch (DataAccessException e) {
            throw new RetrieveTenantRowFromHeaderException(String.format("failed to Select tenant: name=%s, ", tenantName), e);
        }
    }

    private String getHost(HttpServletRequest req) {
        // return req.getRemoteHost();
        return req.getHeader("host");
    }

    // 参加者を取得する
    private PlayerRow retrievePlayer(Connection tenantDb, String id) throws RetrievePlayerException {
        try {
            PreparedStatement ps = tenantDb.prepareStatement("SELECT * FROM player WHERE id = ?");
            ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
            ps.setString(1, id);
            ResultSet rs = ps.executeQuery();
            if (!rs.isBeforeFirst()) {
                return null;
            }
            return new PlayerRow(
                    rs.getLong("tenant_id"),
                    rs.getString("id"),
                    rs.getString("display_name"),
                    rs.getBoolean("is_disqualified"),
                    new Date(rs.getLong("created_at")),
                    new Date(rs.getLong("updated_at")));
        } catch (SQLException e) {
            throw new RetrievePlayerException(String.format("error Select Player: id=%s, ", id), e);
        }
    }

    // 参加者を認可する
    // 参加者向けAPIで呼ばれる
    private void authorizePlayer(Connection tenantDb, String id) throws AuthorizePlayerException {
        try {
            PlayerRow player = this.retrievePlayer(tenantDb, id);
            if (player == null) {
                throw new AuthorizePlayerException(HttpStatus.UNAUTHORIZED, String.format("player not found: id=%s", id));
            }

            if (player.getIsDisqualified()) {
                throw new AuthorizePlayerException(HttpStatus.FORBIDDEN, String.format("player is disqualified: id=%s", id));
            }
        } catch (RetrievePlayerException e) {
            throw new AuthorizePlayerException(HttpStatus.INTERNAL_SERVER_ERROR, e.getMessage(), e);
        }
    }

    // 大会を取得する
    private CompetitionRow retrieveCompetition(Connection tenantDb, String id) throws RetrieveCompetitionException {
        try {
            PreparedStatement ps = tenantDb.prepareStatement("SELECT * FROM competition WHERE id = ?");
            ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
            ps.setString(1, id);
            ResultSet rs = ps.executeQuery();
            if (!rs.isBeforeFirst()) {
                return null;
            }

            return new CompetitionRow(
                    rs.getLong("tenant_id"),
                    rs.getString("id"),
                    rs.getString("title"),
                    new Date(rs.getLong("finished_at")),
                    new Date(rs.getLong("created_at")),
                    new Date(rs.getLong("updated_at")));
        } catch (SQLException e) {
            throw new RetrieveCompetitionException(String.format("error Select competition: id=%s, ", id), e);
        }
    }

    @PostMapping("/api/admin/tenants/add")
    public SuccessResult tenantsAddHandler(HttpServletRequest req, @RequestParam(name = "name") String name, @RequestParam(name = "display_name") String displayName) {
        Viewer v = this.parseViewer(req);

        if (!v.getTenantName().equals("admin")) {
            // admin: SaaS管理者用の特別なテナント名
            throw new WebException(HttpStatus.NOT_FOUND, String.format("%s has not this API", v.getTenantName()));
        }
        if (!v.getRole().equals(ROLE_ADMIN)) {
            throw new WebException(HttpStatus.FORBIDDEN, "admin role required");
        }

        this.validateTenantName(name);

        Date now = new Date();
        SqlParameterSource source = new MapSqlParameterSource()
                .addValue("name", name)
                .addValue("display_name", displayName)
                .addValue("created_at", now.getTime())
                .addValue("updated_at", now.getTime());
        GeneratedKeyHolder holder = new GeneratedKeyHolder();
        try {
            this.adminDb.update("INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (:name, :display_name, :created_at, :updated_at)", source, holder);
        } catch (DataAccessException e) {
            if (e.getRootCause() instanceof SQLException) {
                SQLException se = (SQLException) e.getRootCause();
                // duplicate entry
                if (se.getErrorCode() == 1062) {
                    throw new WebException(HttpStatus.BAD_REQUEST, "duplicate tenant");
                }
            }
            throw new RuntimeException(String.format("error Insert tenant: name=%s, displayName=%s, createdAt=%s, updatedAt=%s", name, displayName, now, now), e);
        }

        if (holder.getKey() == null || holder.getKey().longValue() == 0L) {
            throw new RuntimeException("error get LastInsertId");
        }

        long tenantId = holder.getKey().longValue();
        // NOTE: 先にadminDBに書き込まれることでこのAPIの処理中に
        //       /api/admin/tenants/billingにアクセスされるとエラーになりそう
        //       ロックなどで対処したほうが良さそう
        this.createTenantDB(tenantId);

        TenantWithBilling twb = new TenantWithBilling();
        twb.setId(String.valueOf(tenantId));
        twb.setName(name);
        twb.setDisplayName(displayName);
        twb.setBillingYen(0L);
        return new SuccessResult(true, new TenantsAddHandlerResult(twb));
    }

    // テナント名が規則に沿っているかチェックする
    private void validateTenantName(String name) {
        Pattern p = Pattern.compile(TENANT_NAME_REG_PATTERN);
        Matcher m = p.matcher(name);
        if (!m.find()) {
            throw new WebException(HttpStatus.BAD_REQUEST, String.format("invalid tenant name: %s", name));
        }
    }

    private BillingReport billingReportByCompetition(Connection tenantDb, long tenantId, String competitionId) throws BillingReportByCompetitionException {
        CompetitionRow comp;
        try {
            comp = this.retrieveCompetition(tenantDb, competitionId);
        } catch (RetrieveCompetitionException e) {
            throw new BillingReportByCompetitionException("error retrieveCompetition: ", e);
        }
        if (comp == null) {
            throw new BillingReportByCompetitionException(String.format("error not found competition id=%s : ", competitionId));
        }

        SqlParameterSource source = new MapSqlParameterSource()
                .addValue("tenant_id", tenantId)
                .addValue("competition_id", comp.getId());
        RowMapper<VisitHistorySummaryRow> mapper = (rs, i) -> {
            VisitHistorySummaryRow row = new VisitHistorySummaryRow();
            row.setPlayerId(rs.getString("player_id"));
            row.setMinCreatedAt(new Date(rs.getLong("min_created_at")));
            return row;
        };

        String sql = "SELECT player_id, MIN(created_at) AS min_created_at FROM visit_history WHERE tenant_id = :tenant_id AND competition_id = :competition_id GROUP BY player_id";
        List<VisitHistorySummaryRow> vhs;
        try {
            vhs = this.adminDb.query(sql, source, mapper);
        } catch (DataAccessException e) {
            throw new BillingReportByCompetitionException(String.format("error Select visit_history: tenantID=%d, competitionID=%s", tenantId, comp.getId()), e);
        }

        Map<String, String> billingMap = new HashMap<>();
        for (VisitHistorySummaryRow vh : vhs) {
            // competition.finished_atよりもあとの場合は、終了後に訪問したとみなして大会開催内アクセス済みとみなさない
            if (this.isValidFinishedAt(comp.getFinishedAt()) && comp.getFinishedAt().before(vh.getMinCreatedAt())) {
                continue;
            }
            billingMap.put(vh.getPlayerId(), "visitor");
        }

        // player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
        synchronized (this) {
            try {
                // スコアを登録した参加者のIDを取得する
                List<String> scoredPlayerIDs = new ArrayList<>();
                PreparedStatement ps = tenantDb.prepareStatement("SELECT DISTINCT(player_id) AS player_id FROM player_score WHERE tenant_id = ? AND competition_id = ?");
                ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
                ps.setLong(1, tenantId);
                ps.setString(2, competitionId);
                ResultSet rs = ps.executeQuery();
                while (rs.next()) {
                    scoredPlayerIDs.add(rs.getString("player_id"));
                }

                for (String pid : scoredPlayerIDs) {
                    // スコアが登録されている参加者
                    billingMap.put(pid, "player");
                }

                // 大会が終了している場合のみ請求金額が確定するので計算する
                long playerCount = 0, visitorCount = 0;
                if (this.isValidFinishedAt(comp.getFinishedAt())) {
                    for (Map.Entry<String, String> entry : billingMap.entrySet()) {
                        switch (entry.getValue()) {
                        case "player":
                            playerCount++;
                            break;
                        case "visitor":
                            visitorCount++;
                            break;
                        }
                    }
                }
                BillingReport br = new BillingReport();
                br.setCompetitionId(comp.getId());
                br.setCompetitionTitle(comp.getTitle());
                br.setPlayerCount(playerCount);
                br.setVisitorCount(visitorCount);
                br.setBillingPlayerYen(100 * playerCount); // スコアを登録した参加者は100円
                br.setBillingVisitorYen(10 * visitorCount); // ランキングを閲覧だけした(スコアを登録していない)参加者は10円
                br.setBillingYen(100 * playerCount + 10 * visitorCount);
                return br;
            } catch (SQLException e) {
                throw new BillingReportByCompetitionException(String.format("error Select count player_score: tenantID=%d, competitionID=%s, ", tenantId, competitionId), e);
            }
        }
    }

    // SaaS管理者用API テナントごとの課金レポートを最大10件、テナントのid降順で取得する
    // GET /api/admin/tenants/billing
    // URL引数beforeを指定した場合、指定した値よりもidが小さいテナントの課金レポートを取得する
    @GetMapping("/api/admin/tenants/billing")
    public SuccessResult tenantsBillingHandler(HttpServletRequest req, @RequestParam(name = "before", required = false) Long beforeId) {
        String host = this.getHost(req);
        if (!host.equals(ISUCON_ADMIN_HOSTNAME)) {
            throw new WebException(HttpStatus.NOT_FOUND, String.format("invalid hostname %s", host));
        }

        Viewer viewer = this.parseViewer(req);
        if (!viewer.getRole().equals(ROLE_ADMIN)) {
            throw new WebException(HttpStatus.FORBIDDEN, ("admin role required"));
        }

        // テナントごとに
        // 大会ごとに
        // scoreに登録されているplayerでアクセスした人 * 100
        // scoreに登録されているplayerでアクセスしていない人 * 50
        // scoreに登録されていないplayerでアクセスした人 * 10
        // を合計したものを
        // テナントの課金とする
        RowMapper<TenantRow> mapper = (rs, i) -> {
            TenantRow row = new TenantRow();
            row.setId(rs.getLong("id"));
            row.setName(rs.getString("name"));
            row.setDisplayName(rs.getString("display_name"));
            row.setCreatedAt(new Date(rs.getLong("created_at")));
            row.setUpdatedAt(new Date(rs.getLong("updated_at")));
            return row;
        };

        List<TenantRow> tenantRows;
        try {
            tenantRows = this.adminDb.query("SELECT * FROM tenant ORDER BY id DESC", mapper);
        } catch (DataAccessException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error Select tenant: ", e);
        }

        List<TenantWithBilling> tenantBillings = new ArrayList<>();
        for (TenantRow t : tenantRows) {
            if (beforeId != null && beforeId != 0 && beforeId <= t.getId()) {
                continue;
            }
            TenantWithBilling tb = new TenantWithBilling();
            tb.setId(String.valueOf(t.getId()));
            tb.setName(t.getName());
            tb.setDisplayName(t.getDisplayName());

            try (Connection tenantDb = this.connectToTenantDB(t.getId()); PreparedStatement ps = tenantDb.prepareStatement("SELECT * FROM competition WHERE tenant_id=?");) {
                ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
                ps.setLong(1, t.getId());
                ResultSet rs = ps.executeQuery();

                List<CompetitionRow> cs = new ArrayList<>();
                while (rs.next()) {
                    cs.add(new CompetitionRow(
                            rs.getLong("tenant_id"),
                            rs.getString("id"),
                            rs.getString("title"),
                            new Date(rs.getLong("finished_at")),
                            new Date(rs.getLong("created_at")),
                            new Date(rs.getLong("updated_at"))));
                }

                for (CompetitionRow comp : cs) {
                    BillingReport report = this.billingReportByCompetition(tenantDb, t.getId(), comp.getId());
                    Long billingYen = tb.getBillingYen() == null ? 0L : tb.getBillingYen();
                    billingYen += report.getBillingYen();
                    tb.setBillingYen(billingYen);
                }
                tenantBillings.add(tb);
            } catch (DatabaseException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "failed to connectToTenantDb: ", e);
            } catch (SQLException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "failed to Select competition: ", e);
            } catch (BillingReportByCompetitionException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "failed to billingReportByCompetition: ", e);
            }

            if (tenantBillings.size() >= 10) {
                break;
            }
        }
        return new SuccessResult(true, new TenantsBillingHandlerResult(tenantBillings));
    }

    // テナント管理者向けAPI
    // GET /api/organizer/players
    // 参加者一覧を返す
    @GetMapping("/api/organizer/players")
    public SuccessResult playersListHandler(HttpServletRequest req) {
        Viewer v = this.parseViewer(req);
        if (!v.getRole().equals(ROLE_ORGANIZER)) {
            throw new WebException(HttpStatus.FORBIDDEN, "role organizer required");
        }

        Connection tenantDb = null;
        try {
            List<PlayerRow> pls = new ArrayList<>();
            tenantDb = this.connectToTenantDB(v.getTenantId());
            PreparedStatement ps = tenantDb.prepareStatement("SELECT * FROM player WHERE tenant_id=? ORDER BY created_at DESC");
            ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
            ps.setLong(1, v.getTenantId());
            ResultSet rs = ps.executeQuery();

            while (rs.next()) {
                pls.add(new PlayerRow(
                        rs.getLong("tenant_id"),
                        rs.getString("id"),
                        rs.getString("display_name"),
                        rs.getBoolean("is_disqualified"),
                        new Date(rs.getLong("created_at")),
                        new Date(rs.getLong("updated_at"))));
            }

            List<PlayerDetail> pds = new ArrayList<>();
            for (PlayerRow p : pls) {
                pds.add(new PlayerDetail(p.getId(), p.getDisplayName(), p.getIsDisqualified()));
            }

            return new SuccessResult(true, new PlayersListHandlerResult(pds));
        } catch (DatabaseException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDb: ", e);
        } catch (SQLException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error Select player: ", e);
        }
    }

    // テナント管理者向けAPI
    // GET /api/organizer/players/add
    // テナントに参加者を追加する
    @PostMapping("/api/organizer/players/add")
    public SuccessResult playersAddHandler(HttpServletRequest req, @RequestParam(name = "display_name[]") List<String> displayNames) {
        Viewer v = this.parseViewer(req);
        if (!v.getRole().equals(ROLE_ORGANIZER)) {
            throw new WebException(HttpStatus.FORBIDDEN, "role organizer required");
        }

        List<PlayerDetail> pds = new ArrayList<>();
        try (Connection tenantDb = this.connectToTenantDB(v.getTenantId());) {
            for (String displayName : displayNames) {
                String id = this.dispenseID();

                java.sql.Date now = new java.sql.Date(new Date().getTime());
                try (PreparedStatement ps = tenantDb.prepareStatement("INSERT INTO player (id, tenant_id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)")) {
                    ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
                    ps.setString(1, id);
                    ps.setLong(2, v.getTenantId());
                    ps.setString(3, displayName);
                    ps.setBoolean(4, false);
                    ps.setDate(5, now);
                    ps.setDate(6, now);
                    ps.executeUpdate();
                } catch (SQLException e) {
                    throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR,
                            String.format("error Insert player at tenantDB: id=%s, tenantId=%d, displayName=%s, isDisqualified=%b, createdAt=%s, updatedAt=%s, ",
                                    id, v.getTenantId(), displayName, false, now, now),
                            e);
                }

                PlayerRow p = this.retrievePlayer(tenantDb, id);
                pds.add(new PlayerDetail(p.getId(), p.getDisplayName(), p.getIsDisqualified()));
            }
            return new SuccessResult(true, new PlayersAddHandlerResult(pds));
        } catch (DatabaseException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDb: ", e);
        } catch (DispenseIdException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error dispenseID: ", e);
        } catch (RetrievePlayerException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error retrievePlayer: ", e);
        } catch (SQLException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDB: ", e);
        }
    }

    // テナント管理者向けAPI
    // POST /api/organizer/player/{playerId}/disqualified
    // 参加者を失格にする
    @PostMapping("/api/organizer/player/{playerId}/disqualified")
    public SuccessResult playerDisqualifiedHandler(HttpServletRequest req, @PathVariable("playerId") String playerId) {
        Viewer v = this.parseViewer(req);
        if (!v.getRole().equals(ROLE_ORGANIZER)) {
            throw new WebException(HttpStatus.FORBIDDEN, "role organizer required");
        }

        try ( Connection tenantDb = this.connectToTenantDB(v.getTenantId()); PreparedStatement ps = tenantDb.prepareStatement("UPDATE player SET is_disqualified = ?, updated_at = ? WHERE id = ?");) {
            ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
            java.sql.Date now = new java.sql.Date(new Date().getTime());
            ps.setBoolean(1, true);
            ps.setDate(2, now);
            ps.setString(3, playerId);
            ps.executeUpdate();

            PlayerRow p = this.retrievePlayer(tenantDb, playerId);
            if (p == null) {
                throw new WebException(HttpStatus.NOT_FOUND, "player not found");
            }

            return new SuccessResult(true, new PlayerDisqualifiedHandlerResult(new PlayerDetail(p.getId(), p.getDisplayName(), p.getIsDisqualified())));
        } catch (DatabaseException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDb: ", e);
        } catch (SQLException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, String.format("error Update player id=%s: ", playerId), e);
        } catch (RetrievePlayerException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error retrievePlayer: ", e);
        }
    }

    // テナント管理者向けAPI
    // POST /api/organizer/competitions/add
    // 大会を追加する
    @PostMapping("/api/organizer/competitions/add")
    public SuccessResult competitionsAddHandler(HttpServletRequest req, @ModelAttribute(name = "title") String title) {
        Viewer v = this.parseViewer(req);
        if (!v.getRole().equals(ROLE_ORGANIZER)) {
            throw new WebException(HttpStatus.FORBIDDEN, "role organizer required");
        }

        try (Connection tenantDb = this.connectToTenantDB(v.getTenantId()); PreparedStatement ps = tenantDb.prepareStatement("INSERT INTO competition (id, tenant_id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)");) {
            java.sql.Date now = new java.sql.Date(new Date().getTime());
            String id = this.dispenseID();

            ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
            ps.setString(1, id);
            ps.setLong(2, v.getTenantId());
            ps.setString(3, title);
            ps.setDate(4, null);
            ps.setDate(5, now);
            ps.setDate(6, now);
            ps.executeUpdate();

            return new SuccessResult(true, new CompetitionsAddHandlerResult(new CompetitionDetail(id, title, false)));
        } catch (DatabaseException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDb: ", e);
        } catch (DispenseIdException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error dispenseID: ", e);
        } catch (SQLException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error Insert competition: ", e);
        }
    }

    // テナント管理者向けAPI
    // POST /api/organizer/competition/{competitionId}/finish
    // 大会を終了する
    @PostMapping("/api/organizer/competition/{competitionId}/finish")
    public SuccessResult competitionFinishHandler(HttpServletRequest req, @PathVariable("competitionId") String id) {
        Viewer v = this.parseViewer(req);
        if (!v.getRole().equals(ROLE_ORGANIZER)) {
            throw new WebException(HttpStatus.FORBIDDEN, "role organizer required");
        }

        try (Connection tenantDb = this.connectToTenantDB(v.getTenantId());) {
            CompetitionRow cr = this.retrieveCompetition(tenantDb, id);
            if (cr == null) {
                // 存在しない大会
                throw new WebException(HttpStatus.NOT_FOUND, "competition not found ");
            }

            java.sql.Date now = new java.sql.Date(new Date().getTime());
            PreparedStatement ps = tenantDb.prepareStatement("UPDATE competition SET finished_at = ?, updated_at = ? WHERE id = ?");
            ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
            ps.setDate(1, now);
            ps.setDate(2, now);
            ps.setString(3, id);
            ps.executeUpdate();
            return new SuccessResult(true, null);
        } catch (DatabaseException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDb: ", e);
        } catch (SQLException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error Update competition: ", e);
        } catch (RetrieveCompetitionException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error retrieveCompetition: ", e);
        }
    }

    // テナント管理者向けAPI
    // POST /api/organizer/competition/{competitionId}/score
    // 大会のスコアをCSVでアップロードする
    @PostMapping("/api/organizer/competition/{competitionId}/score")
    public SuccessResult competitionScoreHandler(HttpServletRequest req, @PathVariable("competitionId") String competitionId, @RequestParam("scores") MultipartFile multipartFile) {
        Viewer v = this.parseViewer(req);
        if (!v.getRole().equals(ROLE_ORGANIZER)) {
            throw new WebException(HttpStatus.FORBIDDEN, "role organizer required");
        }

        // DELETEしたタイミングで参照が来ると空っぽのランキングになるのでロックする
        synchronized (this) {
            try (Connection tenantDb = this.connectToTenantDB(v.getTenantId());) {
                CompetitionRow comp = this.retrieveCompetition(tenantDb, competitionId);
                if (comp == null) {
                    // 存在しない大会
                    throw new WebException(HttpStatus.NOT_FOUND, "competition not found ");
                }

                if (this.isValidFinishedAt(comp.getFinishedAt())) {
                    throw new WebException(HttpStatus.BAD_REQUEST, String.format("competition is finished: %s", comp.getFinishedAt()));
                }

                if (multipartFile.isEmpty()) {
                    throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error upload file scores");
                }

                BufferedReader r = new BufferedReader(new InputStreamReader(multipartFile.getInputStream(), StandardCharsets.UTF_8));
                String header = r.readLine();
                if (header == null) {
                    throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error r.readLine at header");
                }

                List<String> headers = Arrays.asList(header.split(","));
                if (!headers.equals(Arrays.asList("player_id", "score"))) {
                    throw new WebException(HttpStatus.BAD_REQUEST, "invalid CSV headers");
                }

                String line = null;
                long rowNum = 0L;
                List<PlayerScoreRow> playerScoreRows = new ArrayList<>();
                while ((line = r.readLine()) != null) {
                    rowNum++;
                    List<String> row = Arrays.asList(line.split(","));
                    if (row.size() != 2) {
                        throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, String.format("row must have two columns: %s", line));
                    }
                    String playerId = row.get(0);
                    String scoreStr = row.get(1);

                    PlayerRow p = this.retrievePlayer(tenantDb, playerId);
                    // 存在しない参加者が含まれている
                    if (p == null) {
                        throw new WebException(HttpStatus.BAD_REQUEST, String.format("player not found: %s", playerId));
                    }

                    long score = 0L;
                    score = Long.valueOf(scoreStr);

                    String id = this.dispenseID();
                    Date now = new Date();
                    playerScoreRows.add(new PlayerScoreRow(v.getTenantId(), id, playerId, competitionId, score, rowNum, now, now));
                }

                {
                    PreparedStatement ps = tenantDb.prepareStatement("DELETE FROM player_score WHERE tenant_id = ? AND competition_id = ?");
                    ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
                    ps.setLong(1, v.getTenantId());
                    ps.setString(2, competitionId);
                    ps.execute();
                }

                {
                    for (PlayerScoreRow psr : playerScoreRows) {
                        PreparedStatement ps = tenantDb.prepareStatement("INSERT INTO player_score (id, tenant_id, player_id, competition_id, score, row_num, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)");
                        ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
                        ps.setString(1, psr.getId());
                        ps.setLong(2, psr.getTenantId());
                        ps.setString(3, psr.getPlayerId());
                        ps.setString(4, psr.getCompetitionId());
                        ps.setLong(5, psr.getScore());
                        ps.setLong(6, psr.getRowNum());
                        ps.setDate(7, new java.sql.Date(psr.getCreatedAt().getTime()));
                        ps.setDate(8, new java.sql.Date(psr.getUpdatedAt().getTime()));
                        ps.executeUpdate();
                    }
                }

                return new SuccessResult(true, new ScoreHandlerResult((long) playerScoreRows.size()));
            } catch (DatabaseException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDb: ", e);
            } catch (SQLException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error tenantdb.player_score: ", e);
            } catch (RetrieveCompetitionException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error retrieveCompetition: ", e);
            } catch (IOException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error new BufferdReader: ", e);
            } catch (RetrievePlayerException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error retrievePlayer: ", e);
            } catch (NumberFormatException e) {
                throw new WebException(HttpStatus.BAD_REQUEST, "error Long.valueOf(scoreStr): ", e);
            } catch (DispenseIdException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error dispenseID: ", e);
            }
        }
    }

    // テナント管理者向けAPI
    // GET /api/organizer/billing
    // テナント内の課金レポートを取得する
    @GetMapping("/api/organizer/billing")
    public SuccessResult billingHandler(HttpServletRequest req) {
        Viewer v = this.parseViewer(req);
        if (!v.getRole().equals(ROLE_ORGANIZER)) {
            throw new WebException(HttpStatus.FORBIDDEN, "role organizer required");
        }

        Connection tenantDb = null;
        try {
            tenantDb = this.connectToTenantDB(v.getTenantId());
            PreparedStatement ps = tenantDb.prepareStatement("SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC");
            ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
            ps.setLong(1, v.getTenantId());
            ResultSet rs = ps.executeQuery();

            List<CompetitionRow> cs = new ArrayList<>();
            while (rs.next()) {
                cs.add(new CompetitionRow(
                        rs.getLong("tenant_id"),
                        rs.getString("id"),
                        rs.getString("title"),
                        new Date(rs.getLong("finished_at")),
                        new Date(rs.getLong("created_at")),
                        new Date(rs.getLong("updated_at"))));
            }

            List<BillingReport> tbrs = new ArrayList<>();
            for (CompetitionRow comp : cs) {
                BillingReport report = this.billingReportByCompetition(tenantDb, v.getTenantId(), comp.getId());
                tbrs.add(report);
            }

            return new SuccessResult(true, new BillingHandlerResult(tbrs));
        } catch (DatabaseException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDb: ", e);
        } catch (SQLException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error Select competition: ", e);
        } catch (BillingReportByCompetitionException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error billingReportByCompetition: ", e);
        } catch (NumberFormatException e) {
            throw new WebException(HttpStatus.BAD_REQUEST, "error Long.valueOf(scoreStr): ", e);
        }
    }

    // 参加者向けAPI
    // GET /api/player/player/{playerId}
    // 参加者の詳細情報を取得する
    // 参加者向けAPI
    @GetMapping("/api/player/player/{playerId}")
    public SuccessResult playerHandler(HttpServletRequest req, @PathVariable("playerId") String playerId) {
        Viewer v = this.parseViewer(req);
        if (!v.getRole().equals(ROLE_PLAYER)) {
            throw new WebException(HttpStatus.FORBIDDEN, "role player required");
        }

        // player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
        synchronized (this) {

            try (Connection tenantDb = this.connectToTenantDB(v.getTenantId());) {
                this.authorizePlayer(tenantDb, v.getPlayerId());

                PlayerRow p = this.retrievePlayer(tenantDb, playerId);
                if (p == null) {
                    throw new WebException(HttpStatus.NOT_FOUND, String.format("player not found: %s", playerId));
                }

                List<CompetitionRow> cs = new ArrayList<>();
                {
                    PreparedStatement ps = tenantDb.prepareStatement("SELECT * FROM competition WHERE tenant_id = ? ORDER BY created_at ASC");
                    ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
                    ps.setLong(1, v.getTenantId());
                    ResultSet resultSet = ps.executeQuery();

                    while (resultSet.next()) {
                        cs.add(new CompetitionRow(
                                resultSet.getLong("tenant_id"),
                                resultSet.getString("id"),
                                resultSet.getString("title"),
                                new Date(resultSet.getLong("finished_at")),
                                new Date(resultSet.getLong("created_at")),
                                new Date(resultSet.getLong("updated_at"))));
                    }
                }

                List<PlayerScoreRow> pss = new ArrayList<>();
                for (CompetitionRow c : cs) {
                    // 最後にCSVに登場したスコアを採用する = row_numが一番大きいもの
                    PreparedStatement ps = tenantDb.prepareStatement("SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? AND player_id = ? ORDER BY row_num DESC LIMIT 1");
                    ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
                    ps.setLong(1, v.getTenantId());
                    ps.setString(2, c.getId());
                    ps.setString(3, p.getId());
                    ResultSet rs = ps.executeQuery();
                    if (!rs.isBeforeFirst()) {
                        // 行がない = スコアが記録されてない
                        continue;
                    }
                    pss.add(new PlayerScoreRow(
                            rs.getLong("tenant_id"),
                            rs.getString("id"),
                            rs.getString("player_id"),
                            rs.getString("competition_id"),
                            rs.getLong("score"),
                            rs.getLong("row_num"),
                            new Date(rs.getLong("created_at")),
                            new Date(rs.getLong("updated_at"))));
                }

                List<PlayerScoreDetail> psds = new ArrayList<>();
                for (PlayerScoreRow psr : pss) {
                    CompetitionRow comp = this.retrieveCompetition(tenantDb, psr.getCompetitionId());
                    psds.add(new PlayerScoreDetail(comp.getTitle(), psr.getScore()));
                }

                return new SuccessResult(true, new PlayerHandlerResult(new PlayerDetail(p.getId(), p.getDisplayName(), p.getIsDisqualified()), psds));
            } catch (DatabaseException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDb: ", e);
            } catch (SQLException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error tenantdb SQL: ", e);
            } catch (NumberFormatException e) {
                throw new WebException(HttpStatus.BAD_REQUEST, "error Long.valueOf(scoreStr): ", e);
            } catch (RetrievePlayerException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error retrievePlayer: ", e);
            } catch (AuthorizePlayerException e) {
                throw new WebException(e.getHttpStatus(), e);
            } catch (RetrieveCompetitionException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error retrieveCompetition: ", e);
            }
        }
    }

    // 参加者向けAPI
    // GET /api/player/competition/{competitionId}/ranking
    // 大会ごとのランキングを取得する
    @GetMapping("/api/player/competition/{competitionId}/ranking")
    public SuccessResult competitionRankingHandler(HttpServletRequest req, @PathVariable("competitionId") String competitionId, @RequestParam(name = "rank_after", required = false, defaultValue = "0") Long rankAfter) {
        Viewer v = this.parseViewer(req);
        if (!v.getRole().equals(ROLE_PLAYER)) {
            throw new WebException(HttpStatus.FORBIDDEN, "role player required");
        }

        // player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
        synchronized (this) {
            try (Connection tenantDb = this.connectToTenantDB(v.getTenantId());) {
                this.authorizePlayer(tenantDb, v.getPlayerId());

                // 大会の存在確認
                CompetitionRow comp = this.retrieveCompetition(tenantDb, competitionId);
                if (comp == null) {
                    throw new WebException(HttpStatus.NOT_FOUND, "competition not found ");
                }

                Date now = new Date();
                TenantRow tenant = null;
                {
                    SqlParameterSource source = new MapSqlParameterSource()
                            .addValue("tenant_id", v.getTenantId());
                    RowMapper<TenantRow> mapper = (rs, i) -> {
                        TenantRow row = new TenantRow();
                        row.setId(rs.getLong("id"));
                        row.setName(rs.getString("name"));
                        row.setDisplayName(rs.getString("display_name"));
                        row.setCreatedAt(new Date(rs.getLong("created_at")));
                        row.setUpdatedAt(new Date(rs.getLong("updated_at")));
                        return row;
                    };
                    tenant = this.adminDb.queryForObject("SELECT * FROM tenant WHERE id = :tenant_id", source, mapper);
                }

                {
                    SqlParameterSource source = new MapSqlParameterSource()
                            .addValue("player_id", v.getPlayerId())
                            .addValue("tenant_id", tenant.getId())
                            .addValue("competition_id", competitionId)
                            .addValue("created_at", now.getTime())
                            .addValue("updated_at", now.getTime());
                    String sql = "INSERT INTO visit_history (player_id, tenant_id, competition_id, created_at, updated_at) VALUES (:player_id, :tenant_id, :competition_id, :created_at, :updated_at)";
                    this.adminDb.update(sql, source);
                }

                List<PlayerScoreRow> pss = new ArrayList<>();
                {
                    PreparedStatement ps = tenantDb.prepareStatement("SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? ORDER BY row_num DESC");
                    ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
                    ps.setLong(1, tenant.getId());
                    ps.setString(2, competitionId);
                    ResultSet rs = ps.executeQuery();
                    while (rs.next()) {
                        pss.add(new PlayerScoreRow(
                                rs.getLong("tenant_id"),
                                rs.getString("id"),
                                rs.getString("player_id"),
                                rs.getString("competition_id"),
                                rs.getLong("score"),
                                rs.getLong("row_num"),
                                new Date(rs.getLong("created_at")),
                                new Date(rs.getLong("updated_at"))));
                    }
                }

                List<CompetitionRank> ranks = new ArrayList<>();
                Set<String> scoredPlayerSet = new HashSet<>();
                {
                    for (PlayerScoreRow ps : pss) {
                        // player_scoreが同一player_id内ではrow_numの降順でソートされているので
                        // 現れたのが2回目以降のplayer_idはより大きいrow_numでスコアが出ているとみなせる
                        if (scoredPlayerSet.contains(ps.getPlayerId())) {
                            continue;
                        }
                        scoredPlayerSet.add(ps.getPlayerId());
                        PlayerRow p = this.retrievePlayer(tenantDb, ps.getPlayerId());
                        CompetitionRank competitionRank = new CompetitionRank();
                        competitionRank.setScore(ps.getScore());
                        competitionRank.setPlayerId(p.getId());
                        competitionRank.setPlayerDisplayName(p.getDisplayName());
                        competitionRank.setRowNum(ps.getRowNum());

                        ranks.add(competitionRank);
                    }
                }

                Collections.sort(ranks, new Comparator<CompetitionRank>() {
                    @Override
                    public int compare(CompetitionRank o1, CompetitionRank o2) {
                        if (o1.getScore().longValue() == o2.getScore().longValue()) {
                            return Long.compare(o1.getRowNum(), o2.getRowNum());
                        }
                        return Long.compare(o2.getScore(), o1.getScore());
                    }
                });

                List<CompetitionRank> pagedRanks = new ArrayList<>();
                for (int i = 0; i < ranks.size(); i++) {
                    if (i < rankAfter) {
                        continue;
                    }
                    CompetitionRank rank = ranks.get(i);

                    CompetitionRank competitionRank = new CompetitionRank();
                    competitionRank.setRank(i + 1L);
                    competitionRank.setScore(rank.getScore());
                    competitionRank.setPlayerId(rank.getPlayerId());
                    competitionRank.setPlayerDisplayName(rank.getPlayerDisplayName());
                    pagedRanks.add(competitionRank);

                    if (pagedRanks.size() >= 100) {
                        break;
                    }
                }

                return new SuccessResult(true, new CompetitionRankingHandlerResult(new CompetitionDetail(comp.getId(), comp.getTitle(), this.isValidFinishedAt(comp.getFinishedAt())), pagedRanks));
            } catch (DataAccessException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error admindb SQL: ", e);
            } catch (DatabaseException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDb: ", e);
            } catch (SQLException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error tenantdb SQL: ", e);
            } catch (RetrievePlayerException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error retrievePlayer: ", e);
            } catch (AuthorizePlayerException e) {
                throw new WebException(e.getHttpStatus(), e);
            } catch (RetrieveCompetitionException e) {
                throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error retrieveCompetition: ", e);
            }
        }
    }

    // 参加者向けAPI
    // GET /api/player/competitions
    // 大会の一覧を取得する
    @GetMapping("/api/player/competitions")
    public SuccessResult playerCompetitionsHandler(HttpServletRequest req) {
        Viewer v = this.parseViewer(req);
        if (!v.getRole().equals(ROLE_PLAYER)) {
            throw new WebException(HttpStatus.FORBIDDEN, "role player required");
        }

        try (Connection tenantDb = this.connectToTenantDB(v.getTenantId());) {
            this.authorizePlayer(tenantDb, v.getPlayerId());
            return this.competitionsHandler(v, tenantDb);
        } catch (DatabaseException | SQLException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDb: ", e);
        } catch (AuthorizePlayerException e) {
            throw new WebException(e.getHttpStatus(), e);
        }
    }

    // テナント管理者向けAPI
    // GET /api/organizer/competitions
    // 大会の一覧を取得する
    @GetMapping("/api/organizer/competitions")
    public SuccessResult organizerCompetitionsHandler(HttpServletRequest req) {
        Viewer v = this.parseViewer(req);
        if (!v.getRole().equals(ROLE_ORGANIZER)) {
            throw new WebException(HttpStatus.FORBIDDEN, "role organizer required");
        }

        try (Connection tenantDb = this.connectToTenantDB(v.getTenantId());) {
            return this.competitionsHandler(v, tenantDb);
        } catch (DatabaseException | SQLException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDb: ", e);
        }
    }

    private SuccessResult competitionsHandler(Viewer v, Connection tenantDb) {
        try {
            PreparedStatement ps = tenantDb.prepareStatement("SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC");
            ps.setQueryTimeout(SQLITE_BUSY_TIMEOUT);
            ps.setLong(1, v.getTenantId());
            ResultSet rs = ps.executeQuery();

            List<CompetitionRow> cs = new ArrayList<>();
            while (rs.next()) {
                cs.add(new CompetitionRow(
                        rs.getLong("tenant_id"),
                        rs.getString("id"),
                        rs.getString("title"),
                        new Date(rs.getLong("finished_at")),
                        new Date(rs.getLong("created_at")),
                        new Date(rs.getLong("updated_at"))));
            }

            List<CompetitionDetail> cds = new ArrayList<>();
            for (CompetitionRow comp : cs) {
                cds.add(new CompetitionDetail(comp.getId(), comp.getTitle(), this.isValidFinishedAt(comp.getFinishedAt())));
            }
            return new SuccessResult(true, new CompetitionsHandlerResult(cds));
        } catch (SQLException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error Select competition: ", e);
        }
    }

    // 全ロール及び未認証でも使えるhandler
    @GetMapping("/api/me")
    public SuccessResult meHandler(HttpServletRequest req) {
        TenantRow tenant;
        try {
            tenant = this.retrieveTenantRowFromHeader(req);
        } catch (RetrieveTenantRowFromHeaderException e1) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "retrieveTenantRowFromHeader", e1);
        }
        if (tenant == null) {
            throw new WebException(HttpStatus.UNAUTHORIZED, "tenant not found");
        }

        TenantDetail td = new TenantDetail(tenant.getName(), tenant.getDisplayName());

        Viewer v = null;
        try {
            v = this.parseViewer(req);
        } catch (WebException e) {
            if (e.getHttpStatus() == HttpStatus.UNAUTHORIZED) {
                return new SuccessResult(true, new MeHandlerResult(td, null, v.getRole(), false));
            }
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error parseViewer: ", e);
        }

        if (v.getRole().equals(ROLE_ADMIN) || v.getRole().equals(ROLE_ORGANIZER)) {
            return new SuccessResult(true, new MeHandlerResult(td, null, v.getRole(), true));
        }

        try (Connection tenantDb = this.connectToTenantDB(v.getTenantId());) {
            PlayerRow p = this.retrievePlayer(tenantDb, v.getPlayerId());
            if (p == null) {
                return new SuccessResult(true, new MeHandlerResult(td, null, ROLE_NONE, false));
            }

            return new SuccessResult(true, new MeHandlerResult(td, new PlayerDetail(p.getId(), p.getDisplayName(), p.getIsDisqualified()), v.getRole(), true));
        } catch (DatabaseException | SQLException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error connectToTenantDb: ", e);
        } catch (RetrievePlayerException e) {
            throw new WebException(HttpStatus.INTERNAL_SERVER_ERROR, "error retrievePlayer: ", e);
        }
    }

    private boolean isValidFinishedAt(Date finishedAt) {
        if (finishedAt == null) {
            return false;
        }
        return !finishedAt.equals(new Date(0L));
    }

    /*
     * ベンチマーカー向けAPI POST /initialize ベンチマーカーが起動したときに最初に呼ぶ
     * データベースの初期化などが実行されるため、スキーマを変更した場合などは適宜改変すること
     */
    @PostMapping("/initialize")
    public SuccessResult initializeHandler() {
        try {
            Process p = Runtime.getRuntime().exec(INITIALIZE_SCRIPT);
            p.waitFor();
            p.destroy();

            InitializeHandlerResult res = new InitializeHandlerResult();
            res.setLang("java");
            // 頑張ったポイントやこだわりポイントがあれば書いてください
            // 競技中の最後に計測したものを参照して、講評記事などで使わせていただきます
            res.setAppeal("");

            return new SuccessResult(true, res);
        } catch (IOException | InterruptedException e) {
            throw new RuntimeException(String.format("error Runtime.exec: %s", e.getMessage()));
        }
    }
}
