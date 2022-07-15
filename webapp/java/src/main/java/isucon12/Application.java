package isucon12;

import java.io.BufferedReader;
import java.io.File;
import java.io.IOException;
import java.io.InputStreamReader;
import java.nio.channels.FileChannel;
import java.nio.channels.FileLock;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.nio.file.StandardOpenOption;
import java.security.KeyFactory;
import java.security.NoSuchAlgorithmException;
import java.security.interfaces.RSAPublicKey;
import java.security.spec.InvalidKeySpecException;
import java.security.spec.PKCS8EncodedKeySpec;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.Date;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
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
import org.springframework.http.HttpStatus;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.jdbc.core.namedparam.SqlParameterSource;
import org.springframework.jdbc.support.GeneratedKeyHolder;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.ModelAttribute;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RestController;

import com.auth0.jwt.JWT;
import com.auth0.jwt.JWTVerifier;
import com.auth0.jwt.algorithms.Algorithm;
import com.auth0.jwt.exceptions.JWTVerificationException;
import com.auth0.jwt.interfaces.DecodedJWT;

import isucon12.exception.WebException;
import isucon12.json.InitializeHandlerResult;
import isucon12.json.SuccessResult;
import isucon12.json.TenantsAddHandlerResult;
import isucon12.model.CompetitionRow;
import isucon12.model.PlayerRow;
import isucon12.model.TenantRow;
import isucon12.model.TenantWithBilling;
import isucon12.model.Viewer;

@SpringBootApplication
@RestController
public class Application {
    @Autowired
    private NamedParameterJdbcTemplate adminDb;

    Logger logger = LoggerFactory.getLogger(Application.class);

    private static final String TENANT_DB_SCHEMA_FILE_PATH = "../sql/tenant/10_schema.sql";
    private static final String INITIALIZE_SCRIPT = "/isucon/webapp/sql/init.sh";
    private static final String COOKIE_NAME = "isuports_session";

    private static final String ROLE_ADMIN     = "admin";
    private static final String ROLE_ORGANIZER = "organizer";
    private static final String ROLE_PLAYER    = "player";
    private static final String ROLE_NONE      = "none";

    private static final String TENANT_NAME_REG_PATTERN = "^[a-z][a-z0-9-]{0,61}[a-z0-9]$";

    /*
     * ENV
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

    public Connection connectToTennantDB(long id) {
        String tenantDBPath = this.tenantDBPath(id);
        if (!new File(tenantDBPath).exists()) {
            throw new RuntimeException(String.format("failed to open tenant DB: %s", tenantDBPath));
        }

        try {
            return DriverManager.getConnection(String.format("jdbc:sqlite:%s", tenantDBPath));
        } catch (SQLException e) {
            throw new RuntimeException(String.format("failed to open tenant DB: %s", e.getMessage()));
        }
    }

    public void createTenantDB(long id) {
        String tenantDBPath = this.tenantDBPath(id);

        try {
            Process p = new ProcessBuilder().command("sh", "createTenantDB.sh", tenantDBPath, TENANT_DB_SCHEMA_FILE_PATH).start();
            int exitCode = p.waitFor();
            if (exitCode != 0) {
                InputStreamReader inputStreamReader = new InputStreamReader(p.getErrorStream());
                Stream<String> streamOfString= new BufferedReader(inputStreamReader).lines();
                String message = streamOfString.collect(Collectors.joining());

                throw new RuntimeException(String.format("failed to exec sqlite3 %s < %s, out=%s", tenantDBPath, TENANT_DB_SCHEMA_FILE_PATH, message));
            }
        } catch (IOException | InterruptedException e) {
            throw new RuntimeException(String.format("failed to exec sqlite3 %s < %s, %s", tenantDBPath, TENANT_DB_SCHEMA_FILE_PATH, e));
        }
    }

    private void closeTenantDbConnection(Connection tenantDb) {
        try {
            if (tenantDb != null) {
                tenantDb.close();
            }
        } catch (SQLException e) {
            throw new RuntimeException("failed close connection", e);
        }

    }

    //  システム全体で一意なIDを生成する
    public String dispenseID() {
        String lastErrorString = "";
        GeneratedKeyHolder holder = new GeneratedKeyHolder();
        SqlParameterSource source = new MapSqlParameterSource().addValue("stub", "a");

        for ( int i = 0 ; i < 100 ; i++ ) {
            try {
                this.adminDb.update("REPLACE INTO id_generator (stub) VALUES (:stub);", source, holder);
            } catch (DataAccessException e) {
                if (e.getRootCause() instanceof SQLException) {
                    SQLException se = (SQLException) e.getRootCause();
                    //  deadlock
                    if (se.getErrorCode() == 1213) {
                        lastErrorString = String.format("error REPLACE INTO id_generator: %s", se.getMessage());
                        continue;
                    }
                }
                throw new RuntimeException(String.format("error REPLACE INTO id_generator: %s", e.getMessage()));
            }
            if (holder.getKey() == null) {
                throw new RuntimeException("error get last insert id");
            }
            break;
        }

        if (holder.getKey().longValue() != 0) {
            return String.valueOf(holder.getKey().longValue());
        }
        throw new RuntimeException(lastErrorString);
    }

    public static void main(String[] args) {
        SpringApplication.run(Application.class, args);
    }


    //  parseViewer

    private RSAPublicKey readPublicKeyFromFile(String filePath) {
        try {
            byte[] keyBytes = Files.readAllBytes(Paths.get(filePath));
            PKCS8EncodedKeySpec spec = new PKCS8EncodedKeySpec(keyBytes);
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

    public TenantRow retrieveTenantRowFromHeader(HttpServletRequest req) {
        // JWTに入っているテナント名とHostヘッダのテナント名が一致しているか確認
        String baseHost = ISUCON_BASE_HOSTNAME;
        String tenantName = StringUtils.removeEnd(req.getRemoteHost(), baseHost);

        // SaaS管理者用ドメイン
        if (tenantName.equals("admin")) {
            return new TenantRow("admin", "admin");
        }

        // テナントの存在確認
        SqlParameterSource source = new MapSqlParameterSource().addValue("name", tenantName);
        RowMapper<TenantRow> mapper = (rs, i) -> {
            TenantRow row = new TenantRow();
            row.setName(rs.getString("name"));
            row.setDisplayName(rs.getString("display_name"));
            return row;
        };

        try {
            return adminDb.queryForObject("SELECT * FROM tenant WHERE name = :name", source, mapper);
        } catch (Exception e) {
            throw new RuntimeException(String.format("failed to Select tenant: name=%s, ", tenantName), e);
        }
    }

    //  参加者を取得する
    private PlayerRow retrievePlayer(Connection tenantDb, String id) {
        try {
            PreparedStatement ps = tenantDb.prepareStatement("SELECT * FROM player WHERE id = ?");
            ps.setString(1, id);
            ResultSet rs = ps.executeQuery();
            return new PlayerRow(
                    rs.getLong("tenant_id"),
                    rs.getString("id"),
                    rs.getString("display_name"),
                    rs.getBoolean("is_disqualified"),
                    rs.getDate("created_at"),
                    rs.getDate("updated_at"));
        } catch (SQLException e) {
            throw new RuntimeException(String.format("error Select Player: id=%s, ", id), e);
        }
    }

    // 参加者を認可する
    // 参加者向けAPIで呼ばれる
    private void authorizePlayer(Connection tenantDb, String id) {
        PlayerRow player = this.retrievePlayer(tenantDb, id);
        if (player == null) {
            throw new WebException(HttpStatus.UNAUTHORIZED, String.format("player not found: id=%s", id));
        }

        if (player.getIsDisqualified()) {
            throw new WebException(HttpStatus.FORBIDDEN, String.format("player is disqualified: id=%s", id));
        }
    }

    //  大会を取得する
    private CompetitionRow retrieveCompetition(Connection tenantDb, String id) {
        try {
            PreparedStatement ps = tenantDb.prepareStatement("SELECT * FROM competition WHERE id = ?");
            ps.setString(1, id);
            ResultSet rs = ps.executeQuery();
            return new CompetitionRow(
                    rs.getLong("tenant_id"),
                    rs.getString("id"),
                    rs.getString("title"),
                    rs.getDate("finished_at"),
                    rs.getDate("created_at"),
                    rs.getDate("updated_at"));
        } catch (SQLException e) {
            throw new RuntimeException(String.format("error Select competition: id=%s, ", id), e);
        }
    }

    // 排他ロックのためのファイル名を生成する
    private String lockFilePath(long id) {
        return Paths.get(ISUCON_TENANT_DB_DIR).resolve(String.format("%d.lock", id)).toString();
    }

    //  排他ロックする
    private FileLock flockByTenantID(long tenantId) {
        String p = this.lockFilePath(tenantId);
        File lockfile = new File(p);
        try {
            FileChannel fc = FileChannel.open(lockfile.toPath(), StandardOpenOption.CREATE, StandardOpenOption.WRITE);
            FileLock lock = fc.tryLock();
            if (lock == null) {
                throw new RuntimeException(String.format("error FileChannel.tryLock: path=%s, ", p));
            }
            return lock;
        } catch (IOException e) {
            throw new RuntimeException(String.format("error flockByTenantID: path=%s, ", p), e);
        }
    }

    @PostMapping("/api/admin/tenants/add")
    public SuccessResult tenantsAddHandle(HttpServletRequest req, @ModelAttribute(name = "name") String name, @ModelAttribute(name = "display_name") String displayName) {
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
                .addValue("created_at", now)
                .addValue("updated_at", now);
        GeneratedKeyHolder holder = new GeneratedKeyHolder();
        try {
            int update = this.adminDb.update("INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (:name, :display_name, :created_at, :updated_at)", source, holder);
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
        Pattern p = Pattern.compile(TENANT_NAME_REG_PATTERN); //電話番号
        Matcher m = p.matcher(name);
        if (!m.find()) {
            throw new WebException(HttpStatus.BAD_REQUEST, String.format("invalid tenant name: %s", name));
        }
    }

    /*
     * SaaS管理者用API
     * テナントごとの課金レポートを最大10件、テナントのid降順で取得する
     * GET /api/admin/tenants/billing
     * URL引数beforeを指定した場合、指定した値よりもidが小さいテナントの課金レポートを取得する
     */
    /*
    @GetMapping("/api/admin/tenants/billing")
    public void tenantsBillingHandler(HttpServletRequest req, @RequestParam(name="before", required=false) Long beforeId) {
        String host = req.getRemoteHost();
        if (!host.equals(ISUCON_ADMIN_HOSTNAME)) {
            throw new WebException(HttpStatus.NOT_FOUND, String.format("invalid hostname %s", host));
        }

        Viewer viewer = this.parseViewer(req);
        if (!viewer.getRole().equals(ROLE_ADMIN)) {
            throw new WebException(HttpStatus.FORBIDDEN, ("admin role required"));
        }

        // テナントごとに
        //   大会ごとに
        //     scoreに登録されているplayerでアクセスした人 * 100
        //     scoreに登録されているplayerでアクセスしていない人 * 50
        //     scoreに登録されていないplayerでアクセスした人 * 10
        //   を合計したものを
        // テナントの課金とする
        RowMapper<TenantRow> mapper = (rs, i) -> {
            TenantRow row = new TenantRow();
            row.setId(rs.getLong("id"));
            row.setName(rs.getString("name"));
            row.setDisplayName(rs.getString("display_name"));
            row.setCreatedAt(rs.getDate("created_at"));
            row.setUpdatedAt(rs.getDate("updated_at"));
            return row;
        };

        List<TenantRow> tenantRows = adminDb.query("SELECT * FROM tenant ORDER BY id DESC", mapper);
        for (TenantRow t : tenantRows) {
            if (beforeId != null && beforeId != 0 && beforeId <= t.getId()) {
                continue;
            }
            TenantWithBilling tb = new TenantWithBilling();
            tb.setId(String.valueOf(t.getId()));
            tb.setName(t.getName());
            tb.setDisplayName(t.getDisplayName());

            Connection tenantDb = null;
            try {
                tenantDb = this.connectToTennantDB(t.getId());
                PreparedStatement ps = tenantDb.prepareStatement("SELECT * FROM competition WHERE tenant_id=?");
                ps.setLong(1, t.getId());
                ResultSet rs = ps.executeQuery();

                List<CompetitionRow> cs = new ArrayList<>();
                while (rs.next()) {
                    cs.add(new CompetitionRow(
                            rs.getLong("tenant_id"),
                            rs.getString("id"),
                            rs.getString("title"),
                            rs.getDate("finished_at"),
                            rs.getDate("created_at"),
                            rs.getDate("updated_at")));
                }

                for (CompetitionRow comp : cs) {
                    //                    this.billingRe

                }
            } catch (SQLException e) {
                throw new RuntimeException(String.format("failed to Select competition: ", e));
            } finally {
                this.closeTenantDbConnection(tenantDb);
            }
        }
    }
     */



    // テナント管理者向けAPI - 参加者追加、一覧、失格
    @GetMapping("/api/organizer/players")
    public void playersListHandler() {

    }

    @PostMapping("/api/organizer/players/add")
    public void playersAddHandler() {

    }

    @PostMapping("/api/organizer/player/{playerId}/disqualified")
    public void playerDisqualifiedHandler() {

    }

    // テナント管理者向けAPI - 大会管理
    @PostMapping("/api/organizer/competitions/add")
    public void competitionsAddHandler() {

    }

    @PostMapping("/api/organizer/competition/:competition_id/finish")
    public void competitionFinishHandler() {

    }

    @PostMapping("/api/organizer/competition/:competition_id/score")
    public void competitionScoreHandler() {

    }

    @GetMapping("/api/organizer/billing")
    public void billingHandler() {

    }

    @GetMapping("/api/organizer/competitions")
    public void organizerCompetitionsHandler() {

    }

    // 参加者向けAPI
    @GetMapping("/api/player/player/:player_id")
    public void playerHandler() {

    }

    @GetMapping("/api/player/competition/:competition_id/ranking")
    public void competitionRankingHandler() {

    }

    @GetMapping("/api/player/competitions")
    public void playerCompetitionsHandler() {

    }

    // 全ロール及び未認証でも使えるhandler
    @GetMapping("/api/me")
    public void meHandler() {

    }

    /*
     * ベンチマーカー向けAPI
     * POST /initialize
     * ベンチマーカーが起動したときに最初に呼ぶ
     * データベースの初期化などが実行されるため、スキーマを変更した場合などは適宜改変すること
     */
    @PostMapping("/initialize")
    public SuccessResult initializeHandler() {
        try {
            Runtime.getRuntime().exec(INITIALIZE_SCRIPT);
            InitializeHandlerResult res = new InitializeHandlerResult();
            res.setLang("java");
            // 頑張ったポイントやこだわりポイントがあれば書いてください
            // 競技中の最後に計測したものを参照して、講評記事などで使わせていただきます
            res.setAppeal("");

            return new SuccessResult(true, res);
        } catch (IOException e) {
            throw new RuntimeException(String.format("error Runtime.exec: %s", e.getMessage()));
        }
    }

    @GetMapping("/error")
    public void error() {
        throw new RuntimeException("error Runtime.exec: %s");
    }

    @GetMapping("/error/web")
    public void errorWeb() {
        throw new WebException(HttpStatus.UNAUTHORIZED, "error WebException");
    }

    @GetMapping("/debug")
    public SuccessResult debug(HttpServletRequest req) throws IOException, InterruptedException {
        Map<String, Object> res = new HashMap<>();

        //        res.put("ISUCON_DB_HOST", ISUCON_DB_HOST);
        //        res.put("ISUCON_DB_PORT", ISUCON_DB_PORT);
        //        res.put("ISUCON_DB_USER", ISUCON_DB_USER);
        //        res.put("ISUCON_DB_PASSWORD", ISUCON_DB_PASSWORD);
        //        res.put("ISUCON_DB_NAME", ISUCON_DB_NAME);
        res.put("ISUCON_TENANT_DB_DIR", ISUCON_TENANT_DB_DIR);
        res.put("SERVER_APP_PORT", SERVER_APP_PORT);
        res.put("ISUCON_JWT_KEY_FILE", ISUCON_JWT_KEY_FILE);
        res.put("ISUCON_BASE_HOSTNAME", ISUCON_BASE_HOSTNAME);
        res.put("ISUCON_ADMIN_HOSTNAME", ISUCON_ADMIN_HOSTNAME);

        String baseHost = ISUCON_BASE_HOSTNAME;
        String tenantName = StringUtils.removeEnd(req.getRemoteHost(), baseHost);

        res.put("tenantName", tenantName);

        RowMapper<TenantRow> mapper = (rs, i) -> {
            TenantRow row = new TenantRow();
            row.setName(rs.getString("name"));
            row.setDisplayName(rs.getString("display_name"));
            return row;
        };

        List<TenantRow> results = adminDb.query("SELECT * FROM tenant", mapper);
        res.put("tenants", results);

        res.put("last insert id", dispenseID());

        Process p = Runtime.getRuntime().exec("ls -l ../tenant_db/");
        p.waitFor();
        InputStreamReader inputStreamReader = new InputStreamReader(p.getInputStream());
        Stream<String> streamOfString= new BufferedReader(inputStreamReader).lines();
        res.put("ls -l", streamOfString.collect(Collectors.joining()));

        Process p2 = Runtime.getRuntime().exec("pwd");
        p2.waitFor();
        InputStreamReader inputStreamReader2 = new InputStreamReader(p2.getInputStream());
        Stream<String> streamOfString2 = new BufferedReader(inputStreamReader2).lines();
        res.put("pwd", streamOfString2.collect(Collectors.joining()));

        this.createTenantDB(130L);

        res.put("tenantDbPath", tenantDBPath(130L));
        res.put("tenantDbConnection", connectToTennantDB(130L).toString());

        this.parseViewer(req);

        return new SuccessResult(true, res);
    }

    // リクエストヘッダをパースしてViewerを返す
    public Viewer parseViewer(HttpServletRequest req) {
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

        TenantRow tenant = retrieveTenantRowFromHeader(req);
        if (tenant == null) {
            throw new WebException(HttpStatus.UNAUTHORIZED, "tenant not found");
        }

        if (tenant.getName().equals("admin") && !role.equals(ROLE_ADMIN)) {
            throw new WebException(HttpStatus.UNAUTHORIZED, "tenant not found");
        }

        if (!tenant.getName().equals(audiences.get(0))) {
            throw new WebException(HttpStatus.UNAUTHORIZED, String.format("invalid token: tenant name is not match with %s: %s", req.getRemoteHost(), token));
        }

        return new Viewer(role, decodedJwt.getSubject(), tenant.getName(), tenant.getId());
    }

}
