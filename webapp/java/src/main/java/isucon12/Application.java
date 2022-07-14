package isucon12;

import java.io.BufferedReader;
import java.io.File;
import java.io.IOException;
import java.io.InputStreamReader;
import java.nio.file.Paths;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.SQLException;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;
import java.util.stream.Stream;

import javax.servlet.http.Cookie;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

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
import org.springframework.web.bind.annotation.ControllerAdvice;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.ResponseBody;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.servlet.mvc.method.annotation.ResponseEntityExceptionHandler;

import isucon12.json.FailureResult;
import isucon12.json.InitializeHandlerResult;
import isucon12.json.SuccessResult;
import isucon12.model.TenantRow;

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

    public static void main(String[] args) {
        SpringApplication.run(Application.class, args);
    }

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
            //            Connection connection = DriverManager.getConnection(String.format("jdbc:sqlite:%s", tenantDBPath));
            //            PreparedStatement ps = connection.prepareStatement("read ?;");
            //            ps.setString(1, TENANT_DB_SCHEMA_FILE_PATH);
            //            ResultSet rs = ps.executeQuery();
        } catch (IOException | InterruptedException e) {
            throw new RuntimeException(String.format("failed to exec sqlite3 %s < %s, %s", tenantDBPath, TENANT_DB_SCHEMA_FILE_PATH, e));
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

    //  エラー処理クラス
    @ControllerAdvice(annotations = {RestController.class})
    public class RestControllerAdvice extends ResponseEntityExceptionHandler {
        Logger logger = LoggerFactory.getLogger(RestControllerAdvice.class);

        @ExceptionHandler(WebException.class)
        @ResponseBody
        public FailureResult handlerWebException(HttpServletRequest req, HttpServletResponse res, WebException e) {
            logger.error("error at {}: status={}: {}", req.getRequestURI(), e.getHttpStatus().value(), e.getErrorMessage());
            res.setStatus(e.getHttpStatus().value());
            return new FailureResult(false, e.getErrorMessage());
        }

        @ExceptionHandler(Throwable.class)
        @ResponseStatus(value = HttpStatus.INTERNAL_SERVER_ERROR)
        @ResponseBody
        public FailureResult handlerException(HttpServletRequest req, HttpServletResponse res, Throwable t) {
            logger.error("error at {}: {}", req.getRequestURI(), t.getMessage(), t);
            return new FailureResult(false, t.getMessage());
        }

    }

    // リクエストヘッダをパースしてViewerを返す
    public void parseViewer(HttpServletRequest req) {
        if (req.getCookies() == null) {
            throw new WebException(HttpStatus.UNAUTHORIZED, "cookie is null");
        }

        Cookie cookie = Stream.of(req.getCookies())
                .filter(c -> c.getName().equals(COOKIE_NAME))
                .findFirst()
                .orElseThrow(() -> new WebException(HttpStatus.UNAUTHORIZED, String.format("cookie %s is not found", COOKIE_NAME)));


        String token = cookie.getValue();

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
            throw new RuntimeException(e);
        }
    }

    // SaaS管理者向けAPI
    @PostMapping("/api/admin/tenants/add")
    public void tenantsAddHandler() {

    }

    @GetMapping("/api/admin/tenants/billing")
    public void tenantsBillingHandler() {

    }

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

    public class WebException extends RuntimeException {
        private static final long serialVersionUID = -8393550601988146162L;
        private final HttpStatus httpStatus;
        private final String errorMessage;

        public WebException(HttpStatus httpStatus, Throwable cause) {
            this(httpStatus, null, cause);
        }

        public WebException(HttpStatus httpStatus, String errorMessage) {
            this(httpStatus, errorMessage, null);
        }

        public WebException(HttpStatus httpStatus, String errorMessage, Throwable cause) {
            super(cause);
            this.httpStatus = httpStatus;
            this.errorMessage = errorMessage;
        }

        public HttpStatus getHttpStatus() {
            return httpStatus;
        }
        public String getErrorMessage() {
            return errorMessage;
        }
    }
}
