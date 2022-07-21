use actix_multipart::Multipart;
use actix_web::http::StatusCode;
use actix_web::middleware::Logger;
use actix_web::{web, HttpRequest, HttpResponse, ResponseError};
use bytes::BytesMut;
use futures_util::stream::StreamExt as _;
use futures_util::stream::TryStreamExt as _;
use lazy_static::lazy_static;
use regex::Regex;
use serde::{Deserialize, Serialize};
use sqlx::mysql::{MySqlConnectOptions, MySqlDatabaseError};
use sqlx::sqlite::{SqliteConnectOptions, SqliteConnection};
use sqlx::Connection as _;
use std::collections::{HashMap, HashSet};
use std::fmt::Display;
use std::path::PathBuf;
use std::result::Result;
use std::time::{SystemTime, UNIX_EPOCH};
use tokio::fs;
use tracing::error;
use tracing_subscriber::prelude::*;

const TENANT_DB_SCHEMA_FILE_PATH: &str = "../sql/tenant/10_schema.sql";
const INITIALIZE_SCRIPT: &str = "../sql/init.sh";
const COOKIE_NAME: &str = "isuports_session";

const ROLE_ADMIN: &str = "admin";
const ROLE_ORGANIZER: &str = "organizer";
const ROLE_PLAYER: &str = "player";

lazy_static! {
    // 正しいテナント名の正規表現
    static ref TENANT_NAME_REGEXP: Regex = Regex::new(r"^[a-z][a-z0-9-]{0,61}[a-z0-9]$").unwrap();
}
use std::fmt::{Formatter, Result as FmtResult};
#[derive(Debug, Serialize)]
struct MyError {
    message: String,
    status: u16,
}
impl Display for MyError {
    fn fmt(&self, f: &mut Formatter) -> FmtResult {
        write!(f, "status={}: {}", self.status, self.message)
    }
}

impl ResponseError for MyError {
    fn error_response(&self) -> HttpResponse {
        #[derive(Debug, Serialize)]
        struct FailureResult {
            status: bool,
        }
        const FAILURE_RESULT: FailureResult = FailureResult { status: false };
        error!("{}", self);
        HttpResponse::build(self.status_code()).json(&FAILURE_RESULT)
    }

    fn status_code(&self) -> StatusCode {
        match self.status {
            200 => StatusCode::OK,
            400 => StatusCode::BAD_REQUEST,
            401 => StatusCode::UNAUTHORIZED,
            403 => StatusCode::FORBIDDEN,
            404 => StatusCode::NOT_FOUND,
            500 => StatusCode::INTERNAL_SERVER_ERROR,
            502 => StatusCode::BAD_GATEWAY,
            _ => StatusCode::INTERNAL_SERVER_ERROR,
        }
    }
}

// 環境変数を取得する, なければデフォルト値を返す
fn get_env(key: &str, default: &str) -> String {
    match std::env::var(key) {
        Ok(val) => val,
        Err(_) => default.to_string(),
    }
}

// テナントDBのパスを返す
fn tenant_db_path(id: i64) -> PathBuf {
    let tenant_db_dir = get_env("ISUCON_TENANT_DB_DIR", "../tenant_db");
    PathBuf::from(tenant_db_dir).join(format!("{}.db", id))
}

// テナントDBに接続する
async fn connect_to_tenant_db(id: i64) -> sqlx::Result<SqliteConnection> {
    let p = tenant_db_path(id);

    let conn = SqliteConnection::connect_with(&SqliteConnectOptions::new().filename(p)).await?;
    Ok(conn)
}

// テナントDBを新規に作成する
async fn create_tenant_db(id: i64) {
    let p = tenant_db_path(id);
    tokio::process::Command::new("sh")
        .arg("-c")
        .arg(format!(
            "sqlite3 {} < {}",
            p.display(),
            TENANT_DB_SCHEMA_FILE_PATH
        ))
        .output()
        .await
        .unwrap_or_else(|_| {
            panic!(
                "failed to exec sqlite3 {} < {}",
                p.to_str().unwrap(),
                TENANT_DB_SCHEMA_FILE_PATH
            )
        });
}

// システム全体で一意なIDを生成する
async fn dispense_id(pool: &sqlx::MySqlPool) -> Result<String, sqlx::Error> {
    let mut last_err = None;
    for _ in 1..100 {
        match sqlx::query("REPLACE INTO id_generator (stub) VALUES (?);")
            .bind("a")
            .execute(pool)
            .await
        {
            Ok(ret) => return Ok(format!("{:x}", ret.last_insert_id())),
            Err(e) => {
                if let Some(database_error) = e.as_database_error() {
                    if let Some(merr) = database_error.try_downcast_ref::<MySqlDatabaseError>() {
                        if merr.number() == 1213 {
                            // deadlock
                            last_err = Some(e);
                            continue;
                        }
                    }
                }
                return Err(e);
            }
        }
    }

    Err(last_err.unwrap())
}

#[actix_web::main]
pub async fn main() -> std::io::Result<()> {
    if std::env::var_os("RUST_LOG").is_none() {
        std::env::set_var("RUST_LOG", "info,sqlx=warn");
    }

    let default_env_filter = tracing_subscriber::EnvFilter::from_default_env();
    if let Ok(sql_trace_file) = std::env::var("ISUCON_SQL_TRACE_FILE") {
        // sqliteのクエリログを出力する設定
        // 環境変数 ISUCON_SQL_TRACE_FILE を設定すると、そのファイルにクエリログを出力する
        // 未設定なら出力しない
        tracing_subscriber::registry()
            .with(
                tracing_subscriber::fmt::layer()
                    .json()
                    .with_writer(
                        std::fs::File::options()
                            .create(true)
                            .append(true)
                            .open(sql_trace_file)?,
                    )
                    .with_target(false)
                    .with_filter(tracing_subscriber::EnvFilter::new("sqlx::query=info")),
            )
            .with(tracing_subscriber::fmt::layer().with_filter(default_env_filter))
            .init();
    } else {
        tracing_subscriber::registry()
            .with(tracing_subscriber::fmt::layer().with_filter(default_env_filter))
            .init();
    }

    let mysql_config = MySqlConnectOptions::new()
        .host(&get_env("ISUCON_DB_HOST", "127.0.0.1"))
        .username(&get_env("ISUCON_DB_USER", "isucon"))
        .password(&get_env("ISUCON_DB_PASSWORD", "isucon"))
        .database(&get_env("ISUCON_DB_NAME", "isuports"))
        .port(
            get_env("ISUCON_DB_PORT", "3306")
                .parse::<u16>()
                .expect("failed to parse port number"),
        );
    let pool = sqlx::mysql::MySqlPoolOptions::new()
        .max_connections(10)
        .connect_with(mysql_config)
        .await
        .expect("failed to connect mysql db");
    let server = actix_web::HttpServer::new(move || {
        let logger = Logger::default();
        let admin_api = web::scope("/admin/tenants")
            .route("/add", web::post().to(tenants_add_handler))
            .route("/billing", web::get().to(tenants_billing_handler));
        let organizer_api = web::scope("/organizer")
            .route("players", web::get().to(players_list_handler))
            .route("players/add", web::post().to(players_add_handler))
            .route(
                "player/{player_id}/disqualified",
                web::post().to(player_disqualified_handler),
            )
            .route("competitions/add", web::post().to(competitions_add_handler))
            .route(
                "competition/{competition_id}/finish",
                web::post().to(competition_finish_handler),
            )
            .route(
                "competition/{competition_id}/score",
                web::post().to(competition_score_handler),
            )
            .route("billing", web::get().to(billing_handler))
            .route(
                "competitions",
                web::get().to(organizer_competitions_handler),
            );
        let player_api = web::scope("/player")
            .route("/player/{player_id}", web::get().to(player_handler))
            .route(
                "/competition/{competition_id}/ranking",
                web::get().to(competition_ranking_handler),
            )
            .route("competitions", web::get().to(player_competitions_handler));

        actix_web::App::new()
            .wrap(logger)
            .wrap_fn(|req, srv| {
                use actix_web::dev::Service as _;
                let fut = srv.call(req);
                async {
                    let mut res = fut.await?;
                    res.headers_mut().insert(
                        actix_web::http::header::CACHE_CONTROL,
                        actix_web::http::header::HeaderValue::from_static("private"),
                    );
                    Ok(res)
                }
            })
            .app_data(web::Data::new(pool.clone()))
            .route("/initialize", web::post().to(initialize_handler))
            .service(
                web::scope("/api")
                    .service(admin_api)
                    .service(organizer_api)
                    .service(player_api)
                    .route("/me", web::get().to(me_handler)),
            )
    });

    if let Some(l) = listenfd::ListenFd::from_env().take_tcp_listener(0)? {
        server.listen(l)?
    } else {
        server.bind((
            "0.0.0.0",
            std::env::var("SERVER_APP_PORT")
                .ok()
                .and_then(|port_str| port_str.parse().ok())
                .unwrap_or(3000),
        ))?
    }
    .run()
    .await
}

// エラー処理関数
// TODO:

#[derive(Debug, Serialize)]
struct SuccessResult<T> {
    status: bool,
    #[serde(bound(serialize = "T: Serialize",))]
    data: Option<T>,
}

#[derive(Debug, Serialize, Deserialize)]
struct FailureResult {
    status: bool,
    message: String,
}

#[derive(Debug, Serialize, Deserialize)]
struct Viewer {
    role: String,
    player_id: String,
    tenant_name: String,
    tenant_id: i64,
}

#[derive(Debug, Deserialize)]
struct Claims {
    sub: Option<String>,
    #[serde(default)]
    aud: Vec<String>,
    role: Option<String>,
}

// リクエストヘッダをパースしてViewerを返す
async fn parse_viewer(pool: &sqlx::MySqlPool, request: HttpRequest) -> Result<Viewer, MyError> {
    let cookie = request.cookie(COOKIE_NAME);
    if cookie.is_none() {
        return Err(MyError {
            status: 401,
            message: format!("cookie {} is not found", COOKIE_NAME),
        });
    }
    let cookie = cookie.unwrap();
    let token_str = cookie.value();
    let key_filename = get_env("ISUCON_JWT_KEY_FILE", "../public.pem");
    let key_src = fs::read(&key_filename).await.map_err(|e| MyError {
        status: 500,
        message: format!("error fs::read: key_filename={}: {}", key_filename, e),
    })?;

    let key = jsonwebtoken::DecodingKey::from_rsa_pem(&key_src).map_err(|e| MyError {
        status: 500,
        message: format!("error jsonwebtoken::DecodingKey::from_rsa_pem: {}", e),
    })?;

    let token = jsonwebtoken::decode::<Claims>(
        token_str,
        &key,
        &jsonwebtoken::Validation::new(jsonwebtoken::Algorithm::RS256),
    );
    if let Err(e) = token {
        return Err(MyError {
            status: 401,
            message: e.to_string(),
        });
    }
    let token = token.unwrap();
    if token.claims.sub.is_none() {
        return Err(MyError {
            status: 401,
            message: format!(
                "invalid token: subject is not found in token: {}",
                token_str
            ),
        });
    }
    if token.claims.role.is_none() {
        return Err(MyError {
            status: 401,
            message: format!("invalid token: role is not found: {}", token_str),
        });
    }
    let tr = token.claims.role.unwrap();
    let role = match tr.as_str() {
        ROLE_ADMIN | ROLE_ORGANIZER | ROLE_PLAYER => tr,
        _ => {
            return Err(MyError {
                status: 401,
                message: format!("invalid token: invalid role: {}", token_str),
            });
        }
    };
    // aud は1要素でテナント名がはいっている
    let aud = token.claims.aud;
    if aud.len() != 1 {
        return Err(MyError {
            status: 401,
            message: format!("invalid token: aud filed is few or too much: {}", token_str),
        });
    }
    let tenant = match retrieve_tenant_row_from_header(pool, request).await {
        Ok(tenant) => tenant,
        _ => {
            return Err(MyError {
                status: 401,
                message: "tenant not found".to_string(),
            })
        }
    };

    if tenant.name == "admin" && role != ROLE_ADMIN {
        return Err(MyError {
            status: 401,
            message: "tenant not found".to_string(),
        });
    }

    if tenant.name != aud[0] {
        return Err(MyError {
            status: 401,
            message: "invalid token: tenant name is not match ".to_string(),
        });
    }

    let viewer = Viewer {
        role,
        player_id: token.claims.sub.unwrap(),
        tenant_name: tenant.name,
        tenant_id: tenant.id,
    };
    Ok(viewer)
}

async fn retrieve_tenant_row_from_header(
    pool: &sqlx::MySqlPool,
    request: HttpRequest,
) -> Result<TenantRow, sqlx::Error> {
    // check if jwt tenant name and host header's tenant name is the same
    let base_host = get_env("ISUCON_BASE_HOSTNAME", ".t.isucon.dev");

    let tenant_name = request
        .headers()
        .get("Host")
        .unwrap()
        .to_str()
        .unwrap()
        .trim_end_matches(&base_host);

    // SaaS管理者用ドメイン
    if tenant_name == "admin" {
        return Ok(TenantRow {
            name: "admin".to_string(),
            display_name: "admin".to_string(),
            id: 0,
            created_at: 0,
            updated_at: 0,
        });
    }
    // テナントの存在確認
    match sqlx::query_as("SELECT * FROM tenant WHERE name = ?")
        .bind(tenant_name)
        .fetch_one(pool)
        .await
    {
        Ok(tenant) => Ok(tenant),
        _ => Err(sqlx::Error::RowNotFound),
    }
}

#[derive(Debug, Serialize, Deserialize, sqlx::FromRow)]
struct TenantRow {
    id: i64,
    name: String,
    display_name: String,
    created_at: i64,
    updated_at: i64,
}

#[derive(Debug, Serialize, Deserialize, sqlx::FromRow)]
struct PlayerRow {
    tenant_id: i64,
    id: String,
    display_name: String,
    is_disqualified: bool,
    created_at: i64,
    updated_at: i64,
}

// 参加者を取得する
async fn retrieve_player(
    tenant_db: &mut SqliteConnection,
    id: &str,
) -> Result<PlayerRow, sqlx::Error> {
    let row: PlayerRow = match sqlx::query_as("SELECT * FROM player WHERE id = ?")
        .bind(id)
        .fetch_one(tenant_db)
        .await
    {
        Ok(row) => row,
        _ => return Err(sqlx::Error::RowNotFound),
    };
    Ok(row)
}

// 参加者を認可する
// 参加者向けAPIで呼ばれる
async fn authorize_player(tenant_db: &mut SqliteConnection, id: &str) -> Result<(), MyError> {
    let player = match retrieve_player(tenant_db, id).await {
        Ok(player) => player,
        Err(sqlx::Error::RowNotFound) => {
            return Err(MyError {
                status: 401,
                message: "player not found".to_string(),
            });
        }
        _ => panic!("error retrieve_tenant_row_from_header at parse_viewer"),
    };
    if player.is_disqualified {
        return Err(MyError {
            status: 403,
            message: "player is disqualified".to_string(),
        });
    }
    Ok(())
}

#[derive(Debug, Serialize, Deserialize, sqlx::FromRow)]
struct CompetitionRow {
    tenant_id: i64,
    id: String,
    title: String,
    finished_at: Option<i64>,
    created_at: i64,
    updated_at: i64,
}

// 大会を取得する
async fn retrieve_competition(
    tenant_db: &mut SqliteConnection,
    id: &str,
) -> Result<CompetitionRow, sqlx::Error> {
    let row: CompetitionRow = match sqlx::query_as("SELECT * FROM competition WHERE id = ?")
        .bind(id)
        .fetch_one(tenant_db)
        .await
    {
        Ok(row) => row,
        _ => return Err(sqlx::Error::RowNotFound),
    };
    Ok(row)
}

#[derive(Debug, Serialize, Deserialize, sqlx::FromRow)]
struct PlayerScoreRow {
    tenant_id: i64,
    id: String,
    player_id: String,
    competition_id: String,
    score: i64,
    row_num: i64,
    created_at: i64,
    updated_at: i64,
}

// 排他ロックのためのファイル名を生成する
fn lock_file_path(id: i64) -> PathBuf {
    let tenant_db_dir = get_env("ISUCON_TENANT_DB_DIR", "../tenant_db");
    PathBuf::from(tenant_db_dir).join(format!("{}.lock", id))
}

#[derive(Debug)]
struct Flock {
    fd: std::os::unix::io::RawFd,
}

// 排他ロックする
async fn flock_by_tenant_id(tenant_id: i64) -> Result<Flock, nix::Error> {
    let p = lock_file_path(tenant_id);
    let fd = nix::fcntl::open(
        &p,
        nix::fcntl::OFlag::O_CREAT | nix::fcntl::OFlag::O_RDONLY,
        nix::sys::stat::Mode::from_bits_truncate(0o600),
    )?;
    tokio::task::spawn_blocking(move || {
        match nix::fcntl::flock(fd, nix::fcntl::FlockArg::LockExclusive) {
            Ok(()) => Ok(Flock { fd }),
            Err(e) => {
                let _ = nix::unistd::close(fd);
                Err(e)
            }
        }
    })
    .await
    .unwrap()
}

impl Drop for Flock {
    fn drop(&mut self) {
        let _ = nix::unistd::close(self.fd);
    }
}

#[derive(Serialize)]
struct TenantsAddHandlerResult {
    tenant: TenantWithBilling,
}

#[derive(Debug, Serialize, Deserialize)]
struct TenantsAddHandlerForm {
    name: String,
    display_name: String,
}

// SaaS管理者用API
// テナントを追加する
// POST /api/admin/tenants/add
async fn tenants_add_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    form: web::Form<TenantsAddHandlerForm>,
) -> actix_web::Result<HttpResponse, MyError> {
    let form = form.into_inner();
    let v: Viewer = parse_viewer(&pool, request).await?;
    if v.tenant_name != "admin" {
        // admin: SaaS管理者用の特別なテナント名
        return Err(MyError {
            status: 404,
            message: format!("{} has not this API", v.tenant_name),
        });
    }
    if v.role != ROLE_ADMIN {
        return Err(MyError {
            status: 403,
            message: "admin role required".to_string(),
        });
    }
    validate_tenant_name(&form.name)?;
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("error now()")
        .as_secs() as i64;

    let insert_res = sqlx::query(
        "INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)",
    )
    .bind(&form.name)
    .bind(&form.display_name)
    .bind(now)
    .bind(now)
    .execute(&**pool)
    .await;
    if let Err(e) = insert_res {
        if let Some(database_error) = e.as_database_error() {
            if let Some(merr) = database_error.try_downcast_ref::<MySqlDatabaseError>() {
                if merr.number() == 1062 {
                    // duplicate entry
                    return Err(MyError {
                        status: 400,
                        message: "duplicate tenant".to_owned(),
                    });
                }
            }
        }
        return Err(MyError {
            status: 500,
            message: e.to_string(),
        });
    }

    let id = insert_res.unwrap().last_insert_id();
    create_tenant_db(id as i64).await;
    let res = TenantsAddHandlerResult {
        tenant: TenantWithBilling {
            id: id.to_string(),
            name: form.name,
            display_name: form.display_name,
            billing_yen: 0,
        },
    };
    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: Some(res),
    }))
}

// テナント名が規則に従っているかチェックする
fn validate_tenant_name(name: &str) -> Result<(), MyError> {
    if TENANT_NAME_REGEXP.is_match(name) {
        Ok(())
    } else {
        Err(MyError {
            status: 400,
            message: format!("invalid tenant name: {}", name),
        })
    }
}

#[derive(Debug, Serialize, Deserialize)]
struct BillingReport {
    competition_id: String,
    competition_title: String,
    player_count: i64,        // スコアを登録した参加者数
    visitor_count: i64,       // ランキングを閲覧だけした(スコアを登録していない)参加者数
    billing_player_yen: i64,  // 請求金額 スコアを登録した参加者分
    billing_visitor_yen: i64, // 請求金額 ランキングを閲覧だけした(スコアを登録していない)参加者分
    billing_yen: i64,         // 合計請求金額
}

#[derive(Debug, Serialize, Deserialize, sqlx::FromRow)]
struct VisitHistoryRow {
    player_id: String,
    tenant_id: i64,
    competition_id: String,
    created_at: i64,
    updated_at: i64,
}

#[derive(Debug, Serialize, Deserialize, sqlx::FromRow)]
struct VisitHistorySummaryRow {
    player_id: String,
    min_created_at: i64,
}

// 大会ごとの課金レポートを計算する
async fn billing_report_by_competition(
    admin_db: &sqlx::MySqlPool,
    tenant_db: &mut SqliteConnection,
    tenant_id: i64,
    competition_id: &str,
) -> sqlx::Result<BillingReport> {
    let comp: CompetitionRow = retrieve_competition(tenant_db, competition_id).await?;
    let vhs: Vec<VisitHistorySummaryRow> = sqlx::query_as(
        "SELECT player_id, MIN(created_at) AS min_created_at FROM visit_history WHERE tenant_id = ? AND competition_id = ? GROUP BY player_id")
        .bind(tenant_id)
        .bind(&comp.id)
        .fetch_all(admin_db).await?;

    let mut billing_map = HashMap::new();
    for vh in vhs {
        // competition.finished_atよりも後の場合は, 終了後に訪問したとみなして大会開催内アクセス済みと見做さない
        if comp.finished_at.is_some() && comp.finished_at.unwrap() < vh.min_created_at {
            continue;
        }
        billing_map.insert(vh.player_id, "visitor");
    }
    // player_scoreを読んでいる時に更新が走ると不整合が起こるのでロックを取得する
    let _fl = flock_by_tenant_id(tenant_id).await.unwrap();

    // スコアを登録した参加者のIDを取得する
    sqlx::query_as(
        "SELECT DISTINCT(player_id) FROM player_score WHERE tenant_id = ? AND competition_id = ?",
    )
    .bind(tenant_id)
    .bind(&comp.id)
    .fetch_all(tenant_db)
    .await?
    .into_iter()
    .for_each(|(ps,): (String,)| {
        billing_map.insert(ps, "player");
    });

    // 大会が終了している場合のみ請求金額が確定するので計算する
    let mut player_count = 0;
    let mut visitor_count = 0;
    if comp.finished_at.is_some() {
        for (_, category) in billing_map {
            if category == "player" {
                player_count += 1;
            } else if category == "visitor" {
                visitor_count += 1;
            }
        }
    }
    Ok(BillingReport {
        competition_id: comp.id,
        competition_title: comp.title,
        player_count,
        visitor_count,
        billing_player_yen: 100 * player_count, // スコアを登録した参加者は100円
        billing_visitor_yen: 10 * visitor_count, // ランキングを閲覧だけした(スコアを登録していない)参加者は10円
        billing_yen: 100 * player_count + 10 * visitor_count,
    })
}

#[derive(Debug, Serialize, Deserialize)]
struct TenantWithBilling {
    id: String,
    name: String,
    display_name: String,
    #[serde(rename = "billing")]
    billing_yen: i64,
}

#[derive(Debug, Serialize)]
struct TenantsBillingHandlerResult {
    tenants: Vec<TenantWithBilling>,
}

#[derive(serde_derive::Serialize, serde_derive::Deserialize)]
struct BillingQuery {
    before: Option<i64>,
}
// SaaS管理者用API
// テナントごとの課金レポートを最大10件, テナントのid降順で取得する
// GET /api/admin/tenants/billing
// URL引数beforeを指定した場合, 指定した値よりもidが小さいテナントの課金レポートを取得する
async fn tenants_billing_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    query: web::Query<BillingQuery>,
    conn: actix_web::dev::ConnectionInfo,
) -> actix_web::Result<HttpResponse, MyError> {
    if conn.host() != get_env("ISUCON_ADMIN_HOSTNAME", "admin.t.isucon.dev") {
        return Err(MyError {
            status: 404,
            message: "invalid hostname".to_string(),
        });
    };
    let v = parse_viewer(&pool, request).await?;
    if v.role != ROLE_ADMIN {
        return Err(MyError {
            status: 403,
            message: "admin role required".to_string(),
        });
    };
    let before_id = query.before.unwrap_or(0);
    // テナントごとに
    //   大会ごとに
    //     scoreが登録されているplayer * 100
    //     scoreが登録されていないplayerでアクセスした人 * 10
    //   を合計したものを
    // テナントの課金とする
    let ts: Vec<TenantRow> = sqlx::query_as("SELECT * FROM tenant ORDER BY id DESC")
        .fetch_all(&**pool)
        .await
        .unwrap();

    let mut tenant_billings = Vec::with_capacity(ts.len());
    for t in ts {
        if before_id != 0 && before_id <= t.id {
            continue;
        }
        let mut tb = TenantWithBilling {
            id: t.id.to_string(),
            name: t.name,
            display_name: t.display_name,
            billing_yen: 0,
        };
        let mut tenant_db = connect_to_tenant_db(t.id).await.unwrap();

        let cs: Vec<CompetitionRow> = sqlx::query_as("SELECT * FROM competition WHERE tenant_id=?")
            .bind(t.id)
            .fetch_all(&mut tenant_db)
            .await
            .unwrap();
        for comp in cs {
            let report = billing_report_by_competition(&pool, &mut tenant_db, t.id, &comp.id)
                .await
                .unwrap();
            tb.billing_yen += report.billing_yen;
        }
        tenant_billings.push(tb);

        if tenant_billings.len() >= 10 {
            break;
        }
    }
    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: Some(TenantsBillingHandlerResult {
            tenants: tenant_billings,
        }),
    }))
}

#[derive(Debug, Serialize, Deserialize)]
struct PlayerDetail {
    id: String,
    display_name: String,
    is_disqualified: bool,
}

#[derive(Debug, Serialize, Deserialize)]
struct PlayersListHandlerResult {
    players: Vec<PlayerDetail>,
}

// テナント管理者向けAPI
// GET /api/organizer/players
// 参加者一覧を返す
async fn players_list_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: actix_web::HttpRequest,
) -> actix_web::Result<HttpResponse, MyError> {
    let v: Viewer = parse_viewer(&pool, request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(MyError {
            status: 403,
            message: "organizer role required".to_string(),
        });
    };
    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    let pls: Vec<PlayerRow> =
        sqlx::query_as("SELECT * FROM player WHERE tenant_id=? ORDER BY created_at DESC")
            .bind(v.tenant_id)
            .fetch_all(&mut tenant_db)
            .await
            .unwrap();
    let mut pds: Vec<PlayerDetail> = Vec::<PlayerDetail>::new();
    for p in pls {
        pds.push(PlayerDetail {
            id: p.id,
            display_name: p.display_name,
            is_disqualified: p.is_disqualified,
        });
    }
    let res = PlayersListHandlerResult { players: pds };
    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: Some(res),
    }))
}

#[derive(Debug, Serialize, Deserialize)]
struct PlayersAddHandlerResult {
    players: Vec<PlayerDetail>,
}

// テナント管理者向けAPI
// GET /api/organizer/players/add
// テナントに参加者を追加する
async fn players_add_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    form_param: web::Form<Vec<(String, String)>>,
) -> actix_web::Result<HttpResponse, MyError> {
    let v: Viewer = parse_viewer(&pool, request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(MyError {
            status: 403,
            message: "organizer role required".to_string(),
        });
    };
    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    let display_names: std::collections::HashSet<String> = form_param
        .into_inner()
        .into_iter()
        .filter_map(|(key, val)| (key == "display_name[]").then(|| val))
        .collect();
    let mut pds = Vec::<PlayerDetail>::new();

    for display_name in display_names {
        let id = dispense_id(&pool).await.unwrap();
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs() as i64;
        sqlx::query("INSERT INTO player (id, tenant_id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)")
            .bind(id.clone())
            .bind(v.tenant_id)
            .bind(display_name)
            .bind(false)
            .bind(now)
            .bind(now)
            .execute(&mut tenant_db)
            .await
            .unwrap();

        let p = retrieve_player(&mut tenant_db, &id).await.unwrap();
        pds.push(PlayerDetail {
            id: p.id,
            display_name: p.display_name,
            is_disqualified: p.is_disqualified,
        });
    }
    let res = PlayersAddHandlerResult { players: pds };
    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: Some(res),
    }))
}

#[derive(Debug, Serialize, Deserialize)]
struct PlayerDisqualifiedHandlerResult {
    player: PlayerDetail,
}

// テナント管理者向けAPI
// POST /api/organizer/player/:player_id/disqualified
// 参加者を失格にする
async fn player_disqualified_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    params: web::Path<(String,)>,
) -> actix_web::Result<HttpResponse, MyError> {
    let v: Viewer = parse_viewer(&pool, request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(MyError {
            status: 403,
            message: "organizer role required".to_string(),
        });
    };
    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    let (player_id,) = params.into_inner();
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;
    sqlx::query("UPDATE player SET is_disqualified = ?, updated_at=? WHERE id = ?")
        .bind(true)
        .bind(now)
        .bind(&player_id)
        .execute(&mut tenant_db)
        .await
        .unwrap();
    let p = match retrieve_player(&mut tenant_db, &player_id).await {
        Ok(p) => p,
        Err(sqlx::Error::RowNotFound) => {
            // 存在しないプレイヤー
            return Err(MyError {
                status: 404,
                message: "player not found".to_string(),
            });
        }
        _ => {
            panic!("panic at retriev player")
        }
    };
    let res = PlayerDisqualifiedHandlerResult {
        player: PlayerDetail {
            id: p.id,
            display_name: p.display_name,
            is_disqualified: p.is_disqualified,
        },
    };
    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: Some(res),
    }))
}

#[derive(Debug, Serialize, Deserialize)]
struct CompetitionDetail {
    id: String,
    title: String,
    is_finished: bool,
}

#[derive(Debug, Serialize, Deserialize)]
struct CompetitionsAddHandlerResult {
    competition: CompetitionDetail,
}

#[derive(Debug, serde_derive::Serialize, serde_derive::Deserialize)]
struct CompetitionAddHandlerFormQuery {
    title: String,
}
// テナント管理者向けAPI
// POST /api/organizer/competitions/add
// 大会を追加する
async fn competitions_add_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    form: web::Form<CompetitionAddHandlerFormQuery>,
) -> actix_web::Result<HttpResponse, MyError> {
    let v: Viewer = parse_viewer(&pool, request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(MyError {
            status: 403,
            message: "organizer role required".to_string(),
        });
    };
    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    let title = form.title.clone();
    let now: i64 = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;
    let id = dispense_id(&pool).await.unwrap();

    sqlx::query("INSERT INTO competition (id, tenant_id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)")
        .bind(&id)
        .bind(v.tenant_id)
        .bind(&title)
        .bind(Option::<i64>::None)
        .bind(now)
        .bind(now)
        .execute(&mut tenant_db)
        .await
        .unwrap();

    let res = CompetitionsAddHandlerResult {
        competition: CompetitionDetail {
            id,
            title,
            is_finished: false,
        },
    };
    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: Some(res),
    }))
}

// テナント管理者向けAPI
// POST /api/organizer/competitions/:competition_id/finish
// 大会を終了する
async fn competition_finish_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    params: web::Path<(String,)>,
) -> actix_web::Result<HttpResponse, MyError> {
    let v: Viewer = parse_viewer(&pool, request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(MyError {
            status: 403,
            message: "organizer role required".to_string(),
        });
    };
    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    let (id,) = params.into_inner();
    if id.is_empty() {
        return Err(MyError {
            status: 400,
            message: "competition id required".to_string(),
        });
    }

    let _ = match retrieve_competition(&mut tenant_db, &id).await {
        Ok(c) => c,
        Err(sqlx::Error::RowNotFound) => {
            return Err(MyError {
                status: 404,
                message: "competition not found".to_string(),
            });
        }
        _ => {
            panic!("panic at retireve_competition");
        }
    };
    let now: i64 = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;

    sqlx::query("UPDATE competition SET finished_at = ?, updated_at=? WHERE id = ?")
        .bind(now)
        .bind(now)
        .bind(id)
        .execute(&mut tenant_db)
        .await
        .unwrap();

    let res = SuccessResult {
        status: true,
        data: Option::<()>::None,
    };
    Ok(HttpResponse::Ok().json(res))
}

#[derive(Debug, Serialize, Deserialize)]
struct ScoreHandlerResult {
    rows: i64,
}

#[derive(Debug, serde_derive::Serialize, serde_derive::Deserialize)]
struct CompetitionScoreHandlerFormQuery {
    competition_id: String,
}
// テナント管理者向けAPI
// POST /api/organizer/competitions/:competition_id/score
// 大会のスコアをCSVでアップロードする
async fn competition_score_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    params: web::Path<(String,)>,
    mut payload: Multipart,
) -> actix_web::Result<HttpResponse, MyError> {
    let v = parse_viewer(&pool, request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(MyError {
            status: 403,
            message: "organizer role required".to_string(),
        });
    };
    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    let (competition_id,) = params.into_inner();
    let comp = match retrieve_competition(&mut tenant_db, &competition_id).await {
        Ok(c) => c,
        Err(sqlx::Error::RowNotFound) => {
            // 存在しない大会
            return Err(MyError {
                status: 404,
                message: "competition not found".to_string(),
            });
        }
        _ => panic!("panic at retrieve_competition"),
    };
    if comp.finished_at.is_some() {
        let res = FailureResult {
            status: false,
            message: "competition is finished".to_string(),
        };
        return Ok(HttpResponse::BadRequest().json(res));
    }
    let mut score_bytes: Option<BytesMut> = None;
    while let Some(item) = payload.next().await {
        let field = item.unwrap();
        let content_disposition = field.content_disposition();
        if content_disposition.get_name().unwrap() == "scores" {
            score_bytes = Some(
                field
                    .map_ok(|chunk| BytesMut::from(&chunk[..]))
                    .try_concat()
                    .await
                    .unwrap(),
            );
            break;
        }
    }
    if score_bytes.is_none() {
        return Err(MyError {
            status: 500,
            message: "scores field does not exist".to_owned(),
        });
    }
    let score_bytes = score_bytes.unwrap();

    let mut rdr = csv::Reader::from_reader(score_bytes.as_ref());
    {
        let headers = rdr.headers().unwrap();
        if headers != ["player_id", "score"].as_slice() {
            return Err(MyError {
                status: 400,
                message: "invalid CSV headers".to_owned(),
            });
        }
    }

    // DELETEしたタイミングで参照が来ると空っぽのランキングになるのでロックする
    let _fl = flock_by_tenant_id(v.tenant_id).await.unwrap();
    let mut player_score_rows = Vec::<PlayerScoreRow>::new();
    for (row_num, row) in rdr.into_records().enumerate() {
        let row = row.unwrap();
        if row.len() != 2 {
            panic!("row must have tow columns");
        };
        let player_id: String = row.clone()[0].to_string();
        let score_str: String = row[1].to_string();
        match retrieve_player(&mut tenant_db, &player_id).await {
            Ok(c) => c,
            Err(sqlx::Error::RowNotFound) => {
                return Err(MyError {
                    status: 400,
                    message: "player not found".to_string(),
                });
            }
            _ => panic!("panic at retireve_player"),
        };
        let score: i64 = match score_str.parse() {
            Ok(s) => s,
            _ => {
                return Err(MyError {
                    status: 400,
                    message: "error parse score_str".to_string(),
                })
            }
        };
        let id = dispense_id(&pool).await.unwrap();
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs() as i64;
        player_score_rows.push(PlayerScoreRow {
            id,
            tenant_id: v.tenant_id,
            player_id: player_id.clone(),
            competition_id: competition_id.clone(),
            score,
            row_num: row_num as i64,
            created_at: now,
            updated_at: now,
        });
    }
    sqlx::query("DELETE FROM player_score WHERE tenant_id = ? AND competition_id = ?")
        .bind(v.tenant_id)
        .bind(&competition_id)
        .execute(&mut tenant_db)
        .await
        .unwrap();

    let rows = player_score_rows.len() as i64;
    for ps in player_score_rows {
        sqlx::query("INSERT INTO player_score (id, tenant_id, player_id, competition_id, score, row_num, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
            .bind(ps.id)
            .bind(ps.tenant_id)
            .bind(ps.player_id)
            .bind(ps.competition_id)
            .bind(ps.score)
            .bind(ps.row_num)
            .bind(ps.created_at)
            .bind(ps.updated_at)
            .execute(&mut tenant_db)
            .await
            .unwrap();
    }
    let res = SuccessResult {
        status: true,
        data: Some(ScoreHandlerResult { rows }),
    };
    Ok(HttpResponse::Ok().json(res))
}

#[derive(Debug, Serialize, Deserialize)]
struct BillingHandlerResult {
    reports: Vec<BillingReport>,
}

// テナント管理者向けAPI
// GET /api/organizer/billing
// テナント内の課題レポートを取得する
async fn billing_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
) -> actix_web::Result<HttpResponse, MyError> {
    let v: Viewer = parse_viewer(&pool, request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(MyError {
            status: 403,
            message: "role organizer required".to_string(),
        });
    };
    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    let cs: Vec<CompetitionRow> =
        sqlx::query_as("SELECT * FROM competition WHERE tenant_id = ? ORDER BY created_at DESC")
            .bind(v.tenant_id)
            .fetch_all(&mut tenant_db)
            .await
            .unwrap();
    let mut tbrs = Vec::<BillingReport>::new();
    for comp in cs {
        let report: BillingReport =
            billing_report_by_competition(&pool, &mut tenant_db, v.tenant_id, &comp.id)
                .await
                .unwrap();
        tbrs.push(report);
    }
    let res = SuccessResult {
        status: true,
        data: Some(BillingHandlerResult { reports: tbrs }),
    };
    Ok(HttpResponse::Ok().json(res))
}

#[derive(Debug, Serialize, Deserialize, sqlx::FromRow)]
struct PlayerScoreDetail {
    competition_title: String,
    score: i64,
}

#[derive(Debug, Serialize, Deserialize)]
struct PlayerHandlerResult {
    player: PlayerDetail,
    scores: Vec<PlayerScoreDetail>,
}

// 参加者向けAPI
// GET /api/player/player/:player_id
// 参加者の詳細情報を取得する
async fn player_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    params: web::Path<(String,)>,
) -> actix_web::Result<HttpResponse, MyError> {
    let v: Viewer = parse_viewer(&pool, request).await?;
    if v.role != ROLE_PLAYER {
        return Err(MyError {
            status: 403,
            message: "role player required".to_string(),
        });
    };
    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    authorize_player(&mut tenant_db, &v.player_id).await?;
    let (player_id,) = params.into_inner();
    let p = match retrieve_player(&mut tenant_db, &player_id).await {
        Ok(p) => p,
        Err(sqlx::Error::RowNotFound) => {
            return Err(MyError {
                status: 404,
                message: "player not found".to_string(),
            });
        }
        _ => {
            panic!("panic at retrieve_player")
        }
    };
    let cs: Vec<CompetitionRow> =
        sqlx::query_as("SELECT * FROM competition ORDER BY created_at ASC")
            .fetch_all(&mut tenant_db)
            .await
            .unwrap();
    // player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
    let _fl = flock_by_tenant_id(v.tenant_id).await.unwrap();
    let mut pss = Vec::<PlayerScoreRow>::new();
    for c in cs {
        let ps: Option<PlayerScoreRow> = sqlx::query_as(
            "SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? AND player_id = ? ORDER BY row_num DESC LIMIT 1")
            .bind(v.tenant_id)
            .bind(c.id.clone())
            .bind(p.id.clone())
            .fetch_optional(&mut tenant_db)
            .await
            .unwrap();
        // 行がない = スコアが記録されてない
        if let Some(ps) = ps {
            pss.push(ps);
        }
    }
    let mut psds = Vec::<PlayerScoreDetail>::new();
    for ps in pss {
        let comp = retrieve_competition(&mut tenant_db, &ps.competition_id)
            .await
            .unwrap();
        psds.push(PlayerScoreDetail {
            competition_title: comp.title,
            score: ps.score,
        });
    }

    let res = SuccessResult {
        status: true,
        data: Some(PlayerHandlerResult {
            player: PlayerDetail {
                id: p.id.clone(),
                display_name: p.display_name.clone(),
                is_disqualified: p.is_disqualified,
            },
            scores: psds,
        }),
    };
    Ok(HttpResponse::Ok().json(res))
}

#[derive(Debug, Serialize, Deserialize)]
struct CompetitionRank {
    rank: i64,
    score: i64,
    player_id: String,
    player_display_name: String,
    #[serde(skip_serializing)]
    row_num: i64, // APIレスポンスのJSONには含まれない
}

#[derive(Debug, Serialize, Deserialize)]
struct CompetitionRankingHandlerResult {
    competition: CompetitionDetail,
    ranks: Vec<CompetitionRank>,
}

#[derive(Debug, Clone, serde_derive::Serialize, serde_derive::Deserialize)]
struct CompetitionRankingHandlerQueryParam {
    rank_after: Option<i64>,
}
// 参加者向けAPI
// GET /api/player/competition/:competition_id/ranking
// 大会ごとのランキングを取得する
async fn competition_ranking_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    params: web::Path<(String,)>,
    query: web::Query<CompetitionRankingHandlerQueryParam>,
) -> actix_web::Result<HttpResponse, MyError> {
    let v: Viewer = parse_viewer(&pool, request).await?;
    if v.role != ROLE_PLAYER {
        return Err(MyError {
            status: 403,
            message: "role player required".to_string(),
        });
    };

    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    authorize_player(&mut tenant_db, &v.player_id).await?;

    let (competition_id,) = params.into_inner();

    // 大会の存在確認
    let competition = match retrieve_competition(&mut tenant_db, &competition_id).await {
        Ok(c) => c,
        Err(sqlx::Error::RowNotFound) => {
            return Err(MyError {
                status: 404,
                message: "competition not found".to_string(),
            });
        }
        _ => panic!("panic at retrieve_competiiton"),
    };
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;
    let tenant: TenantRow = sqlx::query_as("SELECT * FROM tenant WHERE id = ?")
        .bind(v.tenant_id)
        .fetch_one(&**pool)
        .await
        .unwrap();
    sqlx::query("INSERT INTO visit_history (player_id, tenant_id, competition_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)")
    .bind(v.player_id)
    .bind(tenant.id)
    .bind(&competition_id)
    .bind(now)
    .bind(now)
    .execute(&**pool)
    .await.unwrap();

    let rank_after = query.rank_after.unwrap_or(0);

    // player_scoreを読んでいる時に更新が走ると不整合が走るのでロックを取得する
    let _fl = flock_by_tenant_id(v.tenant_id).await.unwrap();
    let pss: Vec<PlayerScoreRow> = sqlx::query_as(
        "SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? ORDER BY row_num DESC")
        .bind(tenant.id)
        .bind(&competition_id)
        .fetch_all(&mut tenant_db)
        .await
        .unwrap();
    let mut ranks = Vec::with_capacity(pss.len());
    let mut scored_player_set = HashSet::with_capacity(pss.len());

    for ps in pss {
        // player_scoreが同一player_id内ではrow_numの降順でソートされているので
        // 現れたのが2回目以降のplayer_idはより大きいrow_numでスコアが出ているとみなせる
        if scored_player_set.contains(&ps.player_id) {
            continue;
        }
        let p = retrieve_player(&mut tenant_db, &ps.player_id)
            .await
            .unwrap();
        scored_player_set.insert(ps.player_id);
        ranks.push(CompetitionRank {
            rank: 0,
            score: ps.score,
            player_id: p.id,
            player_display_name: p.display_name,
            row_num: ps.row_num,
        })
    }
    ranks.sort_by(|a, b| {
        if a.score == b.score {
            a.row_num.cmp(&b.row_num)
        } else {
            b.score.cmp(&a.score)
        }
    });
    let mut paged_ranks = Vec::with_capacity(100);
    for (i, rank) in ranks.into_iter().enumerate() {
        let i = i as i64;
        if i < rank_after {
            continue;
        }
        paged_ranks.push(CompetitionRank {
            rank: i + 1,
            score: rank.score,
            player_id: rank.player_id,
            player_display_name: rank.player_display_name,
            row_num: 0,
        });
        if paged_ranks.len() >= 100 {
            break;
        }
    }
    let res = SuccessResult {
        status: true,
        data: Some(CompetitionRankingHandlerResult {
            competition: CompetitionDetail {
                id: competition.id,
                title: competition.title,
                is_finished: competition.finished_at.is_some(),
            },
            ranks: paged_ranks,
        }),
    };

    Ok(HttpResponse::Ok().json(res))
}

#[derive(Debug, Serialize, Deserialize)]
struct CompetitionsHandlerResult {
    competitions: Vec<CompetitionDetail>,
}

// 参加者向けAPI
// GET /api/player/competitions
// 大会一覧を取得する
async fn player_competitions_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
) -> actix_web::Result<actix_web::HttpResponse, MyError> {
    let v: Viewer = parse_viewer(&pool, request).await?;
    if v.role != ROLE_PLAYER {
        return Err(MyError {
            status: 403,
            message: "role player required".to_string(),
        });
    };
    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    authorize_player(&mut tenant_db, &v.player_id).await?;
    return competitions_handler(Some(v), tenant_db).await;
}

// 主催者向けAPI
// GET /api/organizer/competitions
// 大会一覧を取得する
async fn organizer_competitions_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
) -> actix_web::Result<actix_web::HttpResponse, MyError> {
    let v: Viewer = parse_viewer(&pool, request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(MyError {
            status: 403,
            message: "role organizer required".to_string(),
        });
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    return competitions_handler(Some(v), tenant_db).await;
}

async fn competitions_handler(
    v: Option<Viewer>,
    mut tenant_db: SqliteConnection,
) -> actix_web::Result<actix_web::HttpResponse, MyError> {
    let cs: Vec<CompetitionRow> =
        sqlx::query_as("SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC")
            .bind(v.map(|v| v.tenant_id).unwrap())
            .fetch_all(&mut tenant_db)
            .await
            .unwrap();
    let mut cds = Vec::<CompetitionDetail>::new();
    for comp in cs {
        cds.push(CompetitionDetail {
            id: comp.id,
            title: comp.title,
            is_finished: comp.finished_at.is_some(),
        })
    }
    let res = SuccessResult {
        status: true,
        data: Some(CompetitionsHandlerResult { competitions: cds }),
    };
    Ok(HttpResponse::Ok().json(res))
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct TenantDetail {
    name: String,
    display_name: String,
}

#[derive(Debug, Serialize, Deserialize)]
struct MeHandlerResult {
    tenant: Option<TenantDetail>,
    me: Option<PlayerDetail>,
    role: String,
    logged_in: bool,
}

// 共通API
// GET /api/me
// JWTで認証した結果, テナントやユーザ情報を返す
async fn me_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
) -> actix_web::Result<HttpResponse, MyError> {
    let tenant: TenantRow = match retrieve_tenant_row_from_header(&pool, request.clone()).await {
        Ok(t) => t,
        _ => {
            panic!("retrieve_tenant_row_from_header")
        }
    };
    let td = TenantDetail {
        name: tenant.name,
        display_name: tenant.display_name,
    };
    let v: Viewer = match parse_viewer(&pool, request).await {
        Ok(v) => v,
        Err(e) if e.status == 401 => {
            return Ok(HttpResponse::Ok().json(SuccessResult {
                status: true,
                data: Some(MeHandlerResult {
                    tenant: Some(td),
                    me: None,
                    role: "none".to_string(),
                    logged_in: false,
                }),
            }));
        }
        _ => {
            panic!("panic at parse_viewer")
        }
    };
    if v.role == ROLE_ADMIN || v.role == ROLE_ORGANIZER {
        return Ok(HttpResponse::Ok().json(SuccessResult {
            status: true,
            data: Some(MeHandlerResult {
                tenant: Some(td.clone()),
                me: None,
                role: v.role,
                logged_in: true,
            }),
        }));
    }
    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    let p = match retrieve_player(&mut tenant_db, &v.player_id).await {
        Ok(p) => p,
        Err(sqlx::Error::RowNotFound) => {
            return Ok(HttpResponse::Ok().json(SuccessResult {
                status: true,
                data: Some(MeHandlerResult {
                    tenant: Some(td.clone()),
                    me: None,
                    role: "none".to_string(),
                    logged_in: true,
                }),
            }));
        }
        _ => {
            panic!("panic at retrieve_plaeyer")
        }
    };

    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: Some(MeHandlerResult {
            tenant: Some(td),
            me: Some(PlayerDetail {
                id: p.id,
                display_name: p.display_name,
                is_disqualified: p.is_disqualified,
            }),
            role: v.role,
            logged_in: true,
        }),
    }))
}

#[derive(Debug, Serialize, Deserialize)]
struct InitializeHandlerResult {
    lang: String,
}

// ベンチマーカー向けAPI
// POST /initialize
// ベンチマーカーが起動した時に最初に呼ぶ
// データベースの初期化などが実行されるため, スキーマを変更した場合などは適宜改変すること
async fn initialize_handler() -> actix_web::Result<HttpResponse> {
    let _output = tokio::process::Command::new(INITIALIZE_SCRIPT)
        .output()
        .await
        .expect("error execute initialize script");
    let res = InitializeHandlerResult {
        lang: "rust".to_string(),
    };

    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: Some(res),
    }))
}