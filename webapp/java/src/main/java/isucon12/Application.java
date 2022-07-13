package isucon12;

import java.io.IOException;
import java.util.HashMap;
import java.util.Map;

import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

import org.apache.commons.lang3.StringUtils;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.http.HttpStatus;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.jdbc.core.namedparam.MapSqlParameterSource;
import org.springframework.jdbc.core.namedparam.NamedParameterJdbcTemplate;
import org.springframework.jdbc.core.namedparam.SqlParameterSource;
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

    private static final String TENANT_DB_SCHEMA_FILE_PATH = "../../../../../sql/tenant/10_schema.sql";
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
    @Value("${ISUCON_DB_HOST:127.0.0.1}")
    private String ISUCON_DB_HOST;
    @Value("${ISUCON_DB_PORT:3306}")
    private Integer ISUCON_DB_PORT;
    @Value("${ISUCON_DB_USER:isucon}")
    private String ISUCON_DB_USER;
    @Value("${ISUCON_DB_PASSWORD:isucon}")
    private String ISUCON_DB_PASSWORD;
    @Value("${ISUCON_DB_NAME:isuports}")
    private String ISUCON_DB_NAME;
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

    //  エラー処理クラス
    @ControllerAdvice(annotations = {RestController.class})
    public class RestControllerAdvice extends ResponseEntityExceptionHandler {
        Logger logger = LoggerFactory.getLogger(RestControllerAdvice.class);

        @ExceptionHandler(Throwable.class)
        @ResponseStatus(value = HttpStatus.INTERNAL_SERVER_ERROR)
        @ResponseBody
        public FailureResult handlerException(HttpServletRequest req, HttpServletResponse res, Throwable t) {
            logger.error("error at {}: {}", req.getRequestURI(), t.getMessage());
            return new FailureResult(false, t.getMessage());
        }
    }

    // リクエストヘッダをパースしてViewerを返す
    public void parseViewer() {

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

    @GetMapping("/debug")
    public SuccessResult debug(HttpServletRequest req) {
        Map<String, Object> res = new HashMap<>();

        res.put("ISUCON_DB_HOST", ISUCON_DB_HOST);
        res.put("ISUCON_DB_PORT", ISUCON_DB_PORT);
        res.put("ISUCON_DB_USER", ISUCON_DB_USER);
        res.put("ISUCON_DB_PASSWORD", ISUCON_DB_PASSWORD);
        res.put("ISUCON_DB_NAME", ISUCON_DB_NAME);
        res.put("ISUCON_TENANT_DB_DIR", ISUCON_TENANT_DB_DIR);
        res.put("SERVER_APP_PORT", SERVER_APP_PORT);
        res.put("ISUCON_JWT_KEY_FILE", ISUCON_JWT_KEY_FILE);
        res.put("ISUCON_BASE_HOSTNAME", ISUCON_BASE_HOSTNAME);
        res.put("ISUCON_ADMIN_HOSTNAME", ISUCON_ADMIN_HOSTNAME);

        String baseHost = ISUCON_BASE_HOSTNAME;
        String tenantName = StringUtils.removeEnd(req.getRemoteHost(), baseHost);

        res.put("tenantName", tenantName);
        return new SuccessResult(true, res);
    }

}
