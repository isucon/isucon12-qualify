#![allow(dead_code)]
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
use std::borrow::Cow;
use std::collections::{HashMap, HashSet};
use std::path::PathBuf;
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

#[derive(Debug, thiserror::Error)]
enum Error {
    #[error("flock error: {0}")]
    Flock(#[from] nix::Error),
    #[error("SQLx error: {0}")]
    Sqlx(#[from] sqlx::Error),
    #[error("Multipart error: {0}")]
    Multipart(#[from] actix_multipart::MultipartError),
    #[error("CSV error: {0}")]
    Csv(#[from] csv::Error),
    #[error("{1}")]
    Custom(StatusCode, Cow<'static, str>),
    #[error("{0}")]
    Internal(Cow<'static, str>),
}

// エラー処理
impl ResponseError for Error {
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
        match *self {
            Self::Custom(code, _) => code,
            Self::Flock(_)
            | Self::Sqlx(_)
            | Self::Multipart(_)
            | Self::Csv(_)
            | Self::Internal(_) => StatusCode::INTERNAL_SERVER_ERROR,
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
    SqliteConnection::connect_with(&SqliteConnectOptions::new().filename(p)).await
}

// テナントDBを新規に作成する
async fn create_tenant_db(id: i64) -> Result<(), Error> {
    let p = tenant_db_path(id);
    let output = tokio::process::Command::new("sh")
        .arg("-c")
        .arg(format!(
            "sqlite3 {} < {}",
            p.display(),
            TENANT_DB_SCHEMA_FILE_PATH
        ))
        .output()
        .await
        .map_err(|e| {
            Error::Internal(
                format!(
                    "failed to exec sqlite3 {} < {}: {}",
                    p.display(),
                    TENANT_DB_SCHEMA_FILE_PATH,
                    e
                )
                .into(),
            )
        })?;
    if output.status.success() {
        Ok(())
    } else {
        Err(Error::Internal(
            format!(
                "failed to exec sqlite {} < {}, out={}, err={}",
                p.display(),
                TENANT_DB_SCHEMA_FILE_PATH,
                String::from_utf8_lossy(&output.stdout),
                String::from_utf8_lossy(&output.stderr)
            )
            .into(),
        ))
    }
}

// システム全体で一意なIDを生成する
async fn dispense_id(admin_db: &sqlx::MySqlPool) -> sqlx::Result<String> {
    let mut last_err = None;
    for _ in 1..100 {
        match sqlx::query("REPLACE INTO id_generator (stub) VALUES (?);")
            .bind("a")
            .execute(admin_db)
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
        // SQLのクエリログを出力する設定
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
                .parse()
                .expect("failed to parse port number"),
        );
    let pool = sqlx::mysql::MySqlPoolOptions::new()
        .max_connections(10)
        .connect_with(mysql_config)
        .await
        .expect("failed to connect mysql db");
    let server = actix_web::HttpServer::new(move || {
        // SaaS管理者向けAPI
        let admin_api = web::scope("/admin/tenants")
            .route("/add", web::post().to(tenants_add_handler))
            .route("/billing", web::get().to(tenants_billing_handler));

        // テナント管理者向けAPI - 参加者追加、一覧、失格
        let organizer_api = web::scope("/organizer")
            .route("players", web::get().to(players_list_handler))
            .route("players/add", web::post().to(players_add_handler))
            .route(
                "player/{player_id}/disqualified",
                web::post().to(player_disqualified_handler),
            )
            // テナント管理者向けAPI - 大会管理
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

        // 参加者向けAPI
        let player_api = web::scope("/player")
            .route("/player/{player_id}", web::get().to(player_handler))
            .route(
                "/competition/{competition_id}/ranking",
                web::get().to(competition_ranking_handler),
            )
            .route("competitions", web::get().to(player_competitions_handler));

        actix_web::App::new()
            .wrap(Logger::default())
            .wrap_fn(|req, srv| {
                // 全APIにCache-Control: privateを設定する
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
            // ベンチマーカー向けAPI
            .route("/initialize", web::post().to(initialize_handler))
            .service(
                web::scope("/api")
                    .service(admin_api)
                    .service(organizer_api)
                    .service(player_api)
                    // 全ロール及び未認証でも使えるhandler
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

#[derive(Debug, Serialize)]
struct SuccessResult<T> {
    status: bool,
    data: T,
}

#[derive(Debug, Serialize)]
struct FailureResult {
    status: bool,
    message: String,
}

#[derive(Debug)]
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
async fn parse_viewer(admin_db: &sqlx::MySqlPool, request: &HttpRequest) -> Result<Viewer, Error> {
    let cookie = request.cookie(COOKIE_NAME);
    if cookie.is_none() {
        return Err(Error::Custom(
            StatusCode::UNAUTHORIZED,
            format!("cookie {} is not found", COOKIE_NAME).into(),
        ));
    }
    let cookie = cookie.unwrap();
    let token_str = cookie.value();

    let key_filename = get_env("ISUCON_JWT_KEY_FILE", "../public.pem");
    let key_src = fs::read(&key_filename).await.map_err(|e| {
        Error::Internal(format!("error fs::read: key_filename={}: {}", key_filename, e).into())
    })?;

    let key = jsonwebtoken::DecodingKey::from_rsa_pem(&key_src).map_err(|e| {
        Error::Internal(format!("error jsonwebtoken::DecodingKey::from_rsa_pem: {}", e).into())
    })?;

    let token = jsonwebtoken::decode::<Claims>(
        token_str,
        &key,
        &jsonwebtoken::Validation::new(jsonwebtoken::Algorithm::RS256),
    );
    if let Err(e) = token {
        return Err(Error::Custom(
            StatusCode::UNAUTHORIZED,
            format!("error jsonwebtoken::decode: {}", e).into(),
        ));
    }
    let token = token.unwrap();

    if token.claims.sub.is_none() {
        return Err(Error::Custom(
            StatusCode::UNAUTHORIZED,
            format!(
                "invalid token: subject is not found in token: {}",
                token_str
            )
            .into(),
        ));
    }

    if token.claims.role.is_none() {
        return Err(Error::Custom(
            StatusCode::UNAUTHORIZED,
            format!("invalid token: role is not found: {}", token_str).into(),
        ));
    }
    let tr = token.claims.role.unwrap();
    let role = match tr.as_str() {
        ROLE_ADMIN | ROLE_ORGANIZER | ROLE_PLAYER => tr,
        _ => {
            return Err(Error::Custom(
                StatusCode::UNAUTHORIZED,
                format!("invalid token: invalid role: {}", token_str).into(),
            ));
        }
    };
    // aud は1要素でテナント名がはいっている
    let aud = token.claims.aud;
    if aud.len() != 1 {
        return Err(Error::Custom(
            StatusCode::UNAUTHORIZED,
            format!("invalid token: aud filed is few or too much: {}", token_str).into(),
        ));
    }
    let tenant = retrieve_tenant_row_from_header(admin_db, &request).await?;
    if tenant.is_none() {
        return Err(Error::Custom(
            StatusCode::UNAUTHORIZED,
            "tenant not found".into(),
        ));
    }
    let tenant = tenant.unwrap();
    if tenant.name == "admin" && role != ROLE_ADMIN {
        return Err(Error::Custom(
            StatusCode::UNAUTHORIZED,
            "tenant not found".into(),
        ));
    }

    if tenant.name != aud[0] {
        return Err(Error::Custom(
            StatusCode::UNAUTHORIZED,
            format!("invalid token: tenant name is not match: {}", token_str).into(),
        ));
    }

    Ok(Viewer {
        role,
        player_id: token.claims.sub.unwrap(),
        tenant_name: tenant.name,
        tenant_id: tenant.id,
    })
}

async fn retrieve_tenant_row_from_header(
    admin_db: &sqlx::MySqlPool,
    request: &HttpRequest,
) -> sqlx::Result<Option<TenantRow>> {
    // JWTに入っているテナント名とHostヘッダのテナント名が一致しているか確認
    let base_host = get_env("ISUCON_BASE_HOSTNAME", ".t.isucon.dev");
    let tenant_name = {
        // await_holding_refcell_ref を避けるために tenant_name を String にしておく
        // https://rust-lang.github.io/rust-clippy/master/index.html#await_holding_refcell_ref
        let conn_info = request.connection_info();
        conn_info.host().trim_end_matches(&base_host).to_owned()
    };

    // SaaS管理者用ドメイン
    if tenant_name == "admin" {
        return Ok(Some(TenantRow {
            name: "admin".to_string(),
            display_name: "admin".to_string(),
            id: 0,
            created_at: 0,
            updated_at: 0,
        }));
    }
    // テナントの存在確認
    sqlx::query_as("SELECT * FROM tenant WHERE name = ?")
        .bind(tenant_name)
        .fetch_optional(admin_db)
        .await
}

#[derive(Debug, sqlx::FromRow)]
struct TenantRow {
    id: i64,
    name: String,
    display_name: String,
    created_at: i64,
    updated_at: i64,
}

#[derive(Debug, sqlx::FromRow)]
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
) -> sqlx::Result<Option<PlayerRow>> {
    sqlx::query_as("SELECT * FROM player WHERE id = ?")
        .bind(id)
        .fetch_optional(tenant_db)
        .await
}

// 参加者を認可する
// 参加者向けAPIで呼ばれる
async fn authorize_player(tenant_db: &mut SqliteConnection, id: &str) -> Result<(), Error> {
    let player = match retrieve_player(tenant_db, id).await? {
        Some(player) => player,
        None => {
            return Err(Error::Custom(
                StatusCode::UNAUTHORIZED,
                "player not found".into(),
            ));
        }
    };
    if player.is_disqualified {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "player is disqualified".into(),
        ));
    }
    Ok(())
}

#[derive(Debug, sqlx::FromRow)]
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
) -> sqlx::Result<Option<CompetitionRow>> {
    sqlx::query_as("SELECT * FROM competition WHERE id = ?")
        .bind(id)
        .fetch_optional(tenant_db)
        .await
}

#[derive(Debug, sqlx::FromRow)]
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

#[derive(Debug, Serialize)]
struct TenantsAddHandlerResult {
    tenant: TenantWithBilling,
}

#[derive(Debug, Deserialize)]
struct TenantsAddHandlerForm {
    name: String,
    display_name: String,
}

// SaaS管理者用API
// テナントを追加する
// POST /api/admin/tenants/add
async fn tenants_add_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    form: web::Form<TenantsAddHandlerForm>,
) -> actix_web::Result<HttpResponse, Error> {
    let form = form.into_inner();
    let v = parse_viewer(&admin_db, &request).await?;
    if v.tenant_name != "admin" {
        // admin: SaaS管理者用の特別なテナント名
        return Err(Error::Custom(
            StatusCode::NOT_FOUND,
            format!("{} has not this API", v.tenant_name).into(),
        ));
    }
    if v.role != ROLE_ADMIN {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "admin role required".into(),
        ));
    }

    validate_tenant_name(&form.name)?;
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;

    let insert_res = sqlx::query(
        "INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)",
    )
    .bind(&form.name)
    .bind(&form.display_name)
    .bind(now)
    .bind(now)
    .execute(&**admin_db)
    .await;
    if let Err(e) = insert_res {
        if let Some(database_error) = e.as_database_error() {
            if let Some(merr) = database_error.try_downcast_ref::<MySqlDatabaseError>() {
                if merr.number() == 1062 {
                    // duplicate entry
                    return Err(Error::Custom(
                        StatusCode::BAD_REQUEST,
                        "duplicate tenant".into(),
                    ));
                }
            }
        }
        return Err(e.into());
    }

    let id = insert_res.unwrap().last_insert_id();
    // NOTE: 先にadminDBに書き込まれることでこのAPIの処理中に
    //       /api/admin/tenants/billingにアクセスされるとエラーになりそう
    //       ロックなどで対処したほうが良さそう
    create_tenant_db(id as i64).await?;
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
        data: res,
    }))
}

// テナント名が規則に沿っているかチェックする
fn validate_tenant_name(name: &str) -> Result<(), Error> {
    if TENANT_NAME_REGEXP.is_match(name) {
        Ok(())
    } else {
        Err(Error::Custom(
            StatusCode::BAD_REQUEST,
            format!("invalid tenant name: {}", name).into(),
        ))
    }
}

#[derive(Debug, Serialize)]
struct BillingReport {
    competition_id: String,
    competition_title: String,
    player_count: i64,        // スコアを登録した参加者数
    visitor_count: i64,       // ランキングを閲覧だけした(スコアを登録していない)参加者数
    billing_player_yen: i64,  // 請求金額 スコアを登録した参加者分
    billing_visitor_yen: i64, // 請求金額 ランキングを閲覧だけした(スコアを登録していない)参加者分
    billing_yen: i64,         // 合計請求金額
}

#[derive(Debug, sqlx::FromRow)]
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
) -> Result<BillingReport, Error> {
    let comp = retrieve_competition(tenant_db, competition_id).await?;
    if comp.is_none() {
        return Err(Error::Internal("error retrieve_competition".into()));
    }
    let comp = comp.unwrap();
    let vhs: Vec<VisitHistorySummaryRow> = sqlx::query_as("SELECT player_id, MIN(created_at) AS min_created_at FROM visit_history WHERE tenant_id = ? AND competition_id = ? GROUP BY player_id")
        .bind(tenant_id)
        .bind(&comp.id)
        .fetch_all(admin_db)
        .await?;

    let mut billing_map = HashMap::new();
    for vh in vhs {
        // competition.finished_atよりもあとの場合は、終了後に訪問したとみなして大会開催内アクセス済みとみなさない
        if comp.finished_at.is_some() && comp.finished_at.unwrap() < vh.min_created_at {
            continue;
        }
        billing_map.insert(vh.player_id, "visitor");
    }

    // player_scoreを読んでいる時に更新が走ると不整合が起こるのでロックを取得する
    let _fl = flock_by_tenant_id(tenant_id).await?;

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
        // スコアが登録されている参加者
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

#[derive(Debug, Serialize)]
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

#[derive(Debug, Deserialize)]
struct TenantsBillingHandlerQuery {
    before: Option<i64>,
}

// SaaS管理者用API
// テナントごとの課金レポートを最大10件、テナントのid降順で取得する
// GET /api/admin/tenants/billing
// URL引数beforeを指定した場合、指定した値よりもidが小さいテナントの課金レポートを取得する
async fn tenants_billing_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    query: web::Query<TenantsBillingHandlerQuery>,
    conn: actix_web::dev::ConnectionInfo,
) -> actix_web::Result<HttpResponse, Error> {
    if conn.host() != get_env("ISUCON_ADMIN_HOSTNAME", "admin.t.isucon.dev") {
        return Err(Error::Custom(
            StatusCode::NOT_FOUND,
            format!("invalid hostname {}", conn.host()).into(),
        ));
    };

    let v = parse_viewer(&admin_db, &request).await?;
    if v.role != ROLE_ADMIN {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "admin role required".into(),
        ));
    };

    let before_id = query.before.unwrap_or(0);
    // テナントごとに
    //   大会ごとに
    //     scoreが登録されているplayer * 100
    //     scoreが登録されていないplayerでアクセスした人 * 10
    //   を合計したものを
    // テナントの課金とする
    let ts: Vec<TenantRow> = sqlx::query_as("SELECT * FROM tenant ORDER BY id DESC")
        .fetch_all(&**admin_db)
        .await?;

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
        let mut tenant_db = connect_to_tenant_db(t.id).await?;
        let cs: Vec<CompetitionRow> = sqlx::query_as("SELECT * FROM competition WHERE tenant_id=?")
            .bind(t.id)
            .fetch_all(&mut tenant_db)
            .await?;
        for comp in cs {
            let report =
                billing_report_by_competition(&admin_db, &mut tenant_db, t.id, &comp.id).await?;
            tb.billing_yen += report.billing_yen;
        }
        tenant_billings.push(tb);

        if tenant_billings.len() >= 10 {
            break;
        }
    }
    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: TenantsBillingHandlerResult {
            tenants: tenant_billings,
        },
    }))
}

#[derive(Debug, Serialize)]
struct PlayerDetail {
    id: String,
    display_name: String,
    is_disqualified: bool,
}

#[derive(Debug, Serialize)]
struct PlayersListHandlerResult {
    players: Vec<PlayerDetail>,
}

// テナント管理者向けAPI
// GET /api/organizer/players
// 参加者一覧を返す
async fn players_list_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: actix_web::HttpRequest,
) -> actix_web::Result<HttpResponse, Error> {
    let v = parse_viewer(&admin_db, &request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "role organizer required".into(),
        ));
    };

    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await?;

    let pls: Vec<PlayerRow> =
        sqlx::query_as("SELECT * FROM player WHERE tenant_id=? ORDER BY created_at DESC")
            .bind(v.tenant_id)
            .fetch_all(&mut tenant_db)
            .await?;
    let mut pds = Vec::new();
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
        data: res,
    }))
}

#[derive(Debug, Serialize)]
struct PlayersAddHandlerResult {
    players: Vec<PlayerDetail>,
}

// テナント管理者向けAPI
// GET /api/organizer/players/add
// テナントに参加者を追加する
async fn players_add_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    form_param: web::Form<Vec<(String, String)>>,
) -> actix_web::Result<HttpResponse, Error> {
    let v = parse_viewer(&admin_db, &request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "role organizer required".into(),
        ));
    };

    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await?;

    let display_names = form_param
        .into_inner()
        .into_iter()
        .filter_map(|(key, val)| (key == "display_name[]").then(|| val));

    let mut pds = Vec::new();
    for display_name in display_names {
        let id = dispense_id(&admin_db).await?;

        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs() as i64;
        sqlx::query("INSERT INTO player (id, tenant_id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)")
            .bind(&id)
            .bind(v.tenant_id)
            .bind(display_name)
            .bind(false)
            .bind(now)
            .bind(now)
            .execute(&mut tenant_db)
            .await?;
        let p = retrieve_player(&mut tenant_db, &id).await?;
        if p.is_none() {
            return Err(Error::Internal("error retrieve_player".into()));
        }
        let p = p.unwrap();
        pds.push(PlayerDetail {
            id: p.id,
            display_name: p.display_name,
            is_disqualified: p.is_disqualified,
        });
    }

    let res = PlayersAddHandlerResult { players: pds };
    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: res,
    }))
}

#[derive(Debug, Serialize)]
struct PlayerDisqualifiedHandlerResult {
    player: PlayerDetail,
}

// テナント管理者向けAPI
// POST /api/organizer/player/:player_id/disqualified
// 参加者を失格にする
async fn player_disqualified_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    params: web::Path<(String,)>,
) -> actix_web::Result<HttpResponse, Error> {
    let v = parse_viewer(&admin_db, &request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "role organizer required".into(),
        ));
    };

    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await?;

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
        .await?;
    let p = retrieve_player(&mut tenant_db, &player_id).await?;
    if p.is_none() {
        // 存在しないプレイヤー
        return Err(Error::Custom(
            StatusCode::NOT_FOUND,
            "player not found".into(),
        ));
    }
    let p = p.unwrap();

    let res = PlayerDisqualifiedHandlerResult {
        player: PlayerDetail {
            id: p.id,
            display_name: p.display_name,
            is_disqualified: p.is_disqualified,
        },
    };
    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: res,
    }))
}

#[derive(Debug, Serialize)]
struct CompetitionDetail {
    id: String,
    title: String,
    is_finished: bool,
}

#[derive(Debug, Serialize)]
struct CompetitionsAddHandlerResult {
    competition: CompetitionDetail,
}

#[derive(Debug, Deserialize)]
struct CompetitionAddHandlerForm {
    title: String,
}

// テナント管理者向けAPI
// POST /api/organizer/competitions/add
// 大会を追加する
async fn competitions_add_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    form: web::Form<CompetitionAddHandlerForm>,
) -> actix_web::Result<HttpResponse, Error> {
    let v = parse_viewer(&admin_db, &request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "role organizer required".into(),
        ));
    };

    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await?;

    let title = form.into_inner().title;

    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;
    let id = dispense_id(&admin_db).await?;

    sqlx::query("INSERT INTO competition (id, tenant_id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)")
        .bind(&id)
        .bind(v.tenant_id)
        .bind(&title)
        .bind(Option::<i64>::None)
        .bind(now)
        .bind(now)
        .execute(&mut tenant_db)
        .await?;

    let res = CompetitionsAddHandlerResult {
        competition: CompetitionDetail {
            id,
            title,
            is_finished: false,
        },
    };
    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: res,
    }))
}

// テナント管理者向けAPI
// POST /api/organizer/competition/:competition_id/finish
// 大会を終了する
async fn competition_finish_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    params: web::Path<(String,)>,
) -> actix_web::Result<HttpResponse, Error> {
    let v: Viewer = parse_viewer(&admin_db, &request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "role organizer required".into(),
        ));
    };

    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await?;

    let (id,) = params.into_inner();
    if retrieve_competition(&mut tenant_db, &id).await?.is_none() {
        // 存在しない大会
        return Err(Error::Custom(
            StatusCode::NOT_FOUND,
            "competition not found".into(),
        ));
    }

    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;
    sqlx::query("UPDATE competition SET finished_at = ?, updated_at=? WHERE id = ?")
        .bind(now)
        .bind(now)
        .bind(id)
        .execute(&mut tenant_db)
        .await?;

    let res = SuccessResult {
        status: true,
        data: (),
    };
    Ok(HttpResponse::Ok().json(res))
}

#[derive(Debug, Serialize)]
struct ScoreHandlerResult {
    rows: i64,
}

#[derive(Debug, Deserialize)]
struct CompetitionScoreHandlerForm {
    competition_id: String,
}

// テナント管理者向けAPI
// POST /api/organizer/competition/:competition_id/score
// 大会のスコアをCSVでアップロードする
async fn competition_score_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    params: web::Path<(String,)>,
    mut payload: Multipart,
) -> actix_web::Result<HttpResponse, Error> {
    let v = parse_viewer(&admin_db, &request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "role organizer required".into(),
        ));
    };

    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await?;

    let (competition_id,) = params.into_inner();
    let comp = match retrieve_competition(&mut tenant_db, &competition_id).await? {
        Some(c) => c,
        None => {
            // 存在しない大会
            return Err(Error::Custom(
                StatusCode::NOT_FOUND,
                "competition not found".into(),
            ));
        }
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
        let field = item?;
        let content_disposition = field.content_disposition();
        if content_disposition.get_name().unwrap() == "scores" {
            score_bytes = Some(
                field
                    .map_ok(|chunk| BytesMut::from(&chunk[..]))
                    .try_concat()
                    .await?,
            );
            break;
        }
    }
    if score_bytes.is_none() {
        return Err(Error::Internal("scores field does not exist".into()));
    }
    let score_bytes = score_bytes.unwrap();

    let mut rdr = csv::Reader::from_reader(score_bytes.as_ref());
    {
        let headers = rdr.headers()?;
        if headers != ["player_id", "score"].as_slice() {
            return Err(Error::Custom(
                StatusCode::BAD_REQUEST,
                "invalid CSV headers".into(),
            ));
        }
    }

    // DELETEしたタイミングで参照が来ると空っぽのランキングになるのでロックする
    let _fl = flock_by_tenant_id(v.tenant_id).await?;
    let mut player_score_rows = Vec::new();
    for (row_num, row) in rdr.into_records().enumerate() {
        let row = row?;
        if row.len() != 2 {
            return Err(Error::Internal(
                format!("row must have tow columns: {:?}", row).into(),
            ));
        };
        let player_id = &row[0];
        let score_str = &row[1];
        if retrieve_player(&mut tenant_db, player_id).await?.is_none() {
            // 存在しない参加者が含まれている
            return Err(Error::Custom(
                StatusCode::BAD_REQUEST,
                format!("player not found: {}", player_id).into(),
            ));
        }
        let score: i64 = match score_str.parse() {
            Ok(s) => s,
            Err(e) => {
                return Err(Error::Custom(
                    StatusCode::BAD_REQUEST,
                    format!("error parse: score_str={}, {}", score_str, e).into(),
                ));
            }
        };
        let id = dispense_id(&admin_db).await?;
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs() as i64;
        player_score_rows.push(PlayerScoreRow {
            id,
            tenant_id: v.tenant_id,
            player_id: player_id.to_owned(),
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
        .await?;

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
            .await?;
    }

    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: ScoreHandlerResult { rows },
    }))
}

#[derive(Debug, Serialize)]
struct BillingHandlerResult {
    reports: Vec<BillingReport>,
}

// テナント管理者向けAPI
// GET /api/organizer/billing
// テナント内の課金レポートを取得する
async fn billing_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
) -> actix_web::Result<HttpResponse, Error> {
    let v = parse_viewer(&admin_db, &request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "role organizer required".into(),
        ));
    };

    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await?;

    let cs: Vec<CompetitionRow> =
        sqlx::query_as("SELECT * FROM competition WHERE tenant_id = ? ORDER BY created_at DESC")
            .bind(v.tenant_id)
            .fetch_all(&mut tenant_db)
            .await?;
    let mut tbrs = Vec::with_capacity(cs.len());
    for comp in cs {
        let report =
            billing_report_by_competition(&admin_db, &mut tenant_db, v.tenant_id, &comp.id).await?;
        tbrs.push(report);
    }

    let res = SuccessResult {
        status: true,
        data: BillingHandlerResult { reports: tbrs },
    };
    Ok(HttpResponse::Ok().json(res))
}

#[derive(Debug, Serialize)]
struct PlayerScoreDetail {
    competition_title: String,
    score: i64,
}

#[derive(Debug, Serialize)]
struct PlayerHandlerResult {
    player: PlayerDetail,
    scores: Vec<PlayerScoreDetail>,
}

// 参加者向けAPI
// GET /api/player/player/:player_id
// 参加者の詳細情報を取得する
async fn player_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    params: web::Path<(String,)>,
) -> actix_web::Result<HttpResponse, Error> {
    let v = parse_viewer(&admin_db, &request).await?;
    if v.role != ROLE_PLAYER {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "role player required".into(),
        ));
    };

    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await?;

    authorize_player(&mut tenant_db, &v.player_id).await?;

    let (player_id,) = params.into_inner();
    let p = match retrieve_player(&mut tenant_db, &player_id).await? {
        Some(p) => p,
        None => {
            return Err(Error::Custom(
                StatusCode::NOT_FOUND,
                "player not found".into(),
            ));
        }
    };
    let cs: Vec<CompetitionRow> =
        sqlx::query_as("SELECT * FROM competition WHERE tenant_id = ? ORDER BY created_at ASC")
            .bind(v.tenant_id)
            .fetch_all(&mut tenant_db)
            .await?;

    // player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
    let _fl = flock_by_tenant_id(v.tenant_id).await?;
    let mut pss = Vec::with_capacity(cs.len());
    for c in cs {
        // 最後にCSVに登場したスコアを採用する = row_numが一番大きいもの
        let ps: Option<PlayerScoreRow> = sqlx::query_as("SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? AND player_id = ? ORDER BY row_num DESC LIMIT 1")
            .bind(v.tenant_id)
            .bind(c.id)
            .bind(&p.id)
            .fetch_optional(&mut tenant_db)
            .await?;
        if let Some(ps) = ps {
            pss.push(ps);
        }
        // 行がない = スコアが記録されてない
    }

    let mut psds = Vec::with_capacity(pss.len());
    for ps in pss {
        let comp = retrieve_competition(&mut tenant_db, &ps.competition_id).await?;
        if comp.is_none() {
            return Err(Error::Internal("error retrieve_competition".into()));
        }
        let comp = comp.unwrap();
        psds.push(PlayerScoreDetail {
            competition_title: comp.title,
            score: ps.score,
        });
    }

    let res = SuccessResult {
        status: true,
        data: PlayerHandlerResult {
            player: PlayerDetail {
                id: p.id,
                display_name: p.display_name,
                is_disqualified: p.is_disqualified,
            },
            scores: psds,
        },
    };
    Ok(HttpResponse::Ok().json(res))
}

#[derive(Debug, Serialize)]
struct CompetitionRank {
    rank: i64,
    score: i64,
    player_id: String,
    player_display_name: String,
    #[serde(skip_serializing)]
    row_num: i64, // APIレスポンスのJSONには含まれない
}

#[derive(Debug, Serialize)]
struct CompetitionRankingHandlerResult {
    competition: CompetitionDetail,
    ranks: Vec<CompetitionRank>,
}

#[derive(Debug, Deserialize)]
struct CompetitionRankingHandlerQuery {
    rank_after: Option<i64>,
}

// 参加者向けAPI
// GET /api/player/competition/:competition_id/ranking
// 大会ごとのランキングを取得する
async fn competition_ranking_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    params: web::Path<(String,)>,
    query: web::Query<CompetitionRankingHandlerQuery>,
) -> actix_web::Result<HttpResponse, Error> {
    let v = parse_viewer(&admin_db, &request).await?;
    if v.role != ROLE_PLAYER {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "role player required".into(),
        ));
    };

    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await?;

    authorize_player(&mut tenant_db, &v.player_id).await?;

    let (competition_id,) = params.into_inner();

    // 大会の存在確認
    let competition = match retrieve_competition(&mut tenant_db, &competition_id).await? {
        Some(c) => c,
        None => {
            return Err(Error::Custom(
                StatusCode::NOT_FOUND,
                "competition not found".into(),
            ));
        }
    };

    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;
    let tenant: TenantRow = sqlx::query_as("SELECT * FROM tenant WHERE id = ?")
        .bind(v.tenant_id)
        .fetch_one(&**admin_db)
        .await?;

    sqlx::query("INSERT INTO visit_history (player_id, tenant_id, competition_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)")
        .bind(v.player_id)
        .bind(tenant.id)
        .bind(&competition_id)
        .bind(now)
        .bind(now)
        .execute(&**admin_db)
        .await?;

    let rank_after = query.rank_after.unwrap_or(0);

    // player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
    let _fl = flock_by_tenant_id(v.tenant_id).await?;
    let pss: Vec<PlayerScoreRow> = sqlx::query_as("SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? ORDER BY row_num DESC")
        .bind(tenant.id)
        .bind(&competition_id)
        .fetch_all(&mut tenant_db)
        .await?;
    let mut ranks = Vec::with_capacity(pss.len());
    let mut scored_player_set = HashSet::with_capacity(pss.len());
    for ps in pss {
        // player_scoreが同一player_id内ではrow_numの降順でソートされているので
        // 現れたのが2回目以降のplayer_idはより大きいrow_numでスコアが出ているとみなせる
        if scored_player_set.contains(&ps.player_id) {
            continue;
        }
        let p = retrieve_player(&mut tenant_db, &ps.player_id).await?;
        if p.is_none() {
            return Err(Error::Internal("error retrieve_player".into()));
        }
        let p = p.unwrap();
        scored_player_set.insert(ps.player_id);
        ranks.push(CompetitionRank {
            rank: 0,
            score: ps.score,
            player_id: p.id,
            player_display_name: p.display_name,
            row_num: ps.row_num,
        })
    }
    ranks.sort_by(|a, b| b.score.cmp(&a.score).then(a.row_num.cmp(&b.row_num)));
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
        data: CompetitionRankingHandlerResult {
            competition: CompetitionDetail {
                id: competition.id,
                title: competition.title,
                is_finished: competition.finished_at.is_some(),
            },
            ranks: paged_ranks,
        },
    };

    Ok(HttpResponse::Ok().json(res))
}

#[derive(Debug, Serialize)]
struct CompetitionsHandlerResult {
    competitions: Vec<CompetitionDetail>,
}

// 参加者向けAPI
// GET /api/player/competitions
// 大会の一覧を取得する
async fn player_competitions_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
) -> actix_web::Result<HttpResponse, Error> {
    let v = parse_viewer(&admin_db, &request).await?;
    if v.role != ROLE_PLAYER {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "role player required".into(),
        ));
    };

    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await?;
    authorize_player(&mut tenant_db, &v.player_id).await?;
    competitions_handler(v, tenant_db).await
}

// テナント管理者向けAPI
// GET /api/organizer/competitions
// 大会の一覧を取得する
async fn organizer_competitions_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
) -> actix_web::Result<HttpResponse, Error> {
    let v = parse_viewer(&admin_db, &request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(Error::Custom(
            StatusCode::FORBIDDEN,
            "role organizer required".into(),
        ));
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await?;
    competitions_handler(v, tenant_db).await
}

async fn competitions_handler(
    v: Viewer,
    mut tenant_db: SqliteConnection,
) -> actix_web::Result<HttpResponse, Error> {
    let cs: Vec<CompetitionRow> =
        sqlx::query_as("SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC")
            .bind(v.tenant_id)
            .fetch_all(&mut tenant_db)
            .await?;
    let mut cds = Vec::with_capacity(cs.len());
    for comp in cs {
        cds.push(CompetitionDetail {
            id: comp.id,
            title: comp.title,
            is_finished: comp.finished_at.is_some(),
        })
    }

    let res = SuccessResult {
        status: true,
        data: CompetitionsHandlerResult { competitions: cds },
    };
    Ok(HttpResponse::Ok().json(res))
}

#[derive(Debug, Serialize)]
struct TenantDetail {
    name: String,
    display_name: String,
}

#[derive(Debug, Serialize)]
struct MeHandlerResult {
    tenant: Option<TenantDetail>,
    me: Option<PlayerDetail>,
    role: String,
    logged_in: bool,
}

// 共通API
// GET /api/me
// JWTで認証した結果、テナントやユーザ情報を返す
async fn me_handler(
    admin_db: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
) -> actix_web::Result<HttpResponse, Error> {
    let tenant = match retrieve_tenant_row_from_header(&admin_db, &request).await? {
        Some(t) => t,
        None => {
            return Err(Error::Internal(
                "error retrieve_tenant_row_from_header".into(),
            ));
        }
    };
    let td = TenantDetail {
        name: tenant.name,
        display_name: tenant.display_name,
    };
    let v = match parse_viewer(&admin_db, &request).await {
        Ok(v) => v,
        Err(e) if e.status_code() == StatusCode::UNAUTHORIZED => {
            return Ok(HttpResponse::Ok().json(SuccessResult {
                status: true,
                data: MeHandlerResult {
                    tenant: Some(td),
                    me: None,
                    role: "none".to_string(),
                    logged_in: false,
                },
            }));
        }
        Err(e) => {
            return Err(Error::Internal(format!("error parse_viewer: {}", e).into()));
        }
    };
    if v.role == ROLE_ADMIN || v.role == ROLE_ORGANIZER {
        return Ok(HttpResponse::Ok().json(SuccessResult {
            status: true,
            data: MeHandlerResult {
                tenant: Some(td),
                me: None,
                role: v.role,
                logged_in: true,
            },
        }));
    }

    let mut tenant_db = connect_to_tenant_db(v.tenant_id).await?;
    let p = match retrieve_player(&mut tenant_db, &v.player_id).await? {
        Some(p) => p,
        None => {
            return Ok(HttpResponse::Ok().json(SuccessResult {
                status: true,
                data: MeHandlerResult {
                    tenant: Some(td),
                    me: None,
                    role: "none".to_string(),
                    logged_in: false,
                },
            }));
        }
    };

    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: MeHandlerResult {
            tenant: Some(td),
            me: Some(PlayerDetail {
                id: p.id,
                display_name: p.display_name,
                is_disqualified: p.is_disqualified,
            }),
            role: v.role,
            logged_in: true,
        },
    }))
}

#[derive(Debug, Serialize)]
struct InitializeHandlerResult {
    lang: &'static str,
}

// ベンチマーカー向けAPI
// POST /initialize
// ベンチマーカーが起動したときに最初に呼ぶ
// データベースの初期化などが実行されるため、スキーマを変更した場合などは適宜改変すること
async fn initialize_handler() -> Result<HttpResponse, Error> {
    let output = tokio::process::Command::new(INITIALIZE_SCRIPT)
        .output()
        .await
        .map_err(|e| Error::Internal(format!("error exec command: {}", e).into()))?;
    if output.status.success() {
        let res = InitializeHandlerResult { lang: "rust" };
        Ok(HttpResponse::Ok().json(SuccessResult {
            status: true,
            data: res,
        }))
    } else {
        Err(Error::Internal(
            format!(
                "error exec command: out={}, err={}",
                String::from_utf8_lossy(&output.stdout),
                String::from_utf8_lossy(&output.stderr)
            )
            .into(),
        ))
    }
}
