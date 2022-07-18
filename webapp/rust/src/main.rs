use actix_multipart::Multipart;
use actix_web::middleware::Logger;
use actix_web::{web, HttpRequest, HttpResponse};
use cookie::Cookie;
use futures_util::stream::StreamExt as _;
use jsonwebtoken;
use lazy_static::lazy_static;
use log::info;
use nix::fcntl::{flock, open, FlockArg, OFlag};
use nix::sys::stat::Mode;
use regex::Regex;
use serde::{Deserialize, Serialize};
use sqlx::mysql::MySqlConnectOptions;
use sqlx::sqlite::SqliteConnectOptions;
use sqlx::{Sqlite, SqlitePool};
use std::collections::{HashMap};
use std::fs;
use std::path::{Path, PathBuf};
use std::result::Result;
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};
use tokio::io::AsyncWriteExt;

const TENANT_DB_SCHEMA_FILE_PATH: &str = "../sql/tenant/10_schema.sql";
const INITIALIZE_SCRIPT: &str = "../sql/init.sh";
const COOKIE_NAME: &str = "isuports_session";

const ROLE_ADMIN: &str = "admin";
const ROLE_ORGANIZER: &str = "organizer";
const ROLE_PLAYER: &str = "player";

lazy_static! {
    // 正しいテナント名の正規表現
    static ref TENANT_NAME_REGEXP: Regex = Regex::new(r"^[a-z][a-z0-9-]{0,61}[a-z0-9]$").unwrap();
    // TODO: static ref ADMIN_DB:

    static ref SQLITE_DRIBER_NAME: String = "sqlite3".to_string();
}

// 環境変数を取得する, なければデフォルト値を返す
fn get_env(key: &str, default: &str) -> String {
    info!("get_env now");
    match std::env::var(key) {
        Ok(val) => val,
        Err(_) => default.to_string(),
    }
}

// テナントDBのパスを返す
fn tenant_db_path(id: i64) -> PathBuf {
    info!("tenant_db_path now");
    let tenant_db_dir = get_env("ISUCON_TENANT_DB_DIR", "/home/isucon/webapp/tenant_db");
    return Path::new(&tenant_db_dir).join(format!("{}.db", id));
}

// テナントDBに接続する
async fn connect_to_tenant_db(id: i64) -> sqlx::Result<SqlitePool> {
    info!("connect to tenant db now: id = {:?}",id);
    let p = tenant_db_path(id);

    let uri = format!("{}", p.to_str().unwrap());
    info!("tenant db uri = {:?}",uri);
    let pool = SqlitePool::connect_with(SqliteConnectOptions::new().filename(uri).create_if_missing(true)).await?;
    info!("created db file now");
    Ok(pool)
    // TODO: sqliteDriverNameを使ってないのをなおす
}
// テナントDBを新規に作成する
async fn create_tenant_db(id: i64) {
    info!("create_tenant_db now");
    let p = tenant_db_path(id);
    tokio::process::Command::new("sh")
        .arg("-c")
        .arg(format!(
            "sqlite3 {} < {}",
            p.to_str().expect("error: to_str()"),
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
async fn dispense_id(pool: web::Data<sqlx::MySqlPool>) -> Result<String, sqlx::Error> {
    info!("dispense id now");
    let mut id: i64 = 0;
    for _ in 1..100 {
        let ret = match sqlx::query("REPLACE INTO id_generator (stub) VALUES (?);")
            .bind("a")
            .execute(pool.as_ref())
            .await
        {
            Ok(ret) => ret,
            _ => break,
        };
        info!("last_insert_id = {:?}", ret.last_insert_id());
        id = ret.last_insert_id().try_into().unwrap();
        break;
    }

    if id != 0 {
        Ok(id.to_string())
    } else {
        Err(sqlx::Error::RowNotFound)
    }
}

#[actix_web::main]
pub async fn main() -> std::io::Result<()> {
    std::env::set_var("RUST_LOG", "info");
    env_logger::init();

    // sqliteのクエリログを出力する設定
    // 環境変数 ISUCON_SQLITE_TRACE_FILEを設定すると, そのファイルにクエリログをJSON形式で出力する
    // 未設定なら出力しない
    // sqltrace.rsを参照
    info!("start");

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
    info!("mysql config are set");
    let pool = sqlx::mysql::MySqlPoolOptions::new()
        .max_connections(10)
        .connect_with(mysql_config)
        .await
        .expect("failed to connect mysql db");
    info!("pool is set");
    let server = actix_web::HttpServer::new(move || {
        let logger = Logger::default();
        let admin_api = web::scope("/admin/tenants")
            .route("/add", web::post().to(tenants_add_handler))
            .route("/billing", web::get().to(tenants_billing_handler));
        let organizer_api = web::scope("/organizer")
            .route("players", web::get().to(players_list_handler))
            .route("players/add", web::post().to(players_add_handler))
            .route(
                "player/{player_id}",
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
    info!("server is running");
    server
        .bind((
            "0.0.0.0",
            std::env::var("SERVER_APP_PORT")
                .ok()
                .and_then(|port_str| port_str.parse().ok())
                .unwrap_or(3000),
        ))
        .expect("failed to start server")
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
    sub: String,
    aud: Vec<String>,
    role: String,
}

// parse request header and return Viewer
async fn parse_viewer(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
) -> Result<Viewer, actix_web::Error> {
    info!("parse viewer now");
    let cookie = request
        .headers()
        .get("cookie")
        .map(|value| value.to_str().unwrap_or_default())
        .unwrap_or_default();

    let c = Cookie::parse(cookie).unwrap();
    let req_jwt = c.value();
    let key_file_name = get_env("ISUCON_JWT_KEY_FILE", "./public.pem");
    let key_src = fs::read_to_string(key_file_name).expect("Something went wrong reading the file");

    let key = jsonwebtoken::DecodingKey::from_rsa_pem(key_src.as_bytes()).unwrap();

    let token = match jsonwebtoken::decode::<Claims>(
        req_jwt,
        &key,
        &jsonwebtoken::Validation::new(jsonwebtoken::Algorithm::RS256),
    ) {
        Ok(token) => token,
        Err(e) if e.kind().to_owned() == jsonwebtoken::errors::ErrorKind::InvalidToken => {
            return Err(actix_web::error::ErrorUnauthorized("failed to parse token"))
        }
        Err(e) if e.kind().to_owned() == jsonwebtoken::errors::ErrorKind::ExpiredSignature => {
            info!("{:?}", e.kind());
            return Err(actix_web::error::ErrorUnauthorized(
                "JWT expired or not signed with the correct key",
            ));
        }
        _ => {
            panic!("failed to parse token");
        }
    };
    if token.claims.sub.is_empty() {
        return Err(actix_web::error::ErrorUnauthorized(
            "invalid token: subject is not found in token",
        ));
    }
    info!("{:?}", token);
    let tr = token.claims.role;
    info!("{:?}", tr);
    let role = match tr.as_str() {
        ROLE_ADMIN => tr.to_string(),
        ROLE_ORGANIZER => tr.to_string(),
        ROLE_PLAYER => tr.to_string(),
        _ => {
            return Err(actix_web::error::ErrorUnauthorized(
                "invalid token: role is not found or invalid role",
            ))
        }
    };
    // aud contains one tenant name
    let aud = token.claims.aud;
    if aud.len() != 1 {
        return Err(actix_web::error::ErrorUnauthorized(
            "invalid token: aud field is few or too much",
        ));
    }
    let tenant = match retrieve_tenant_row_from_header(pool, request).await {
        Ok(tenant) => tenant,
        _ => return Err(actix_web::error::ErrorUnauthorized("tenant not found")),
    };

    if tenant.name == "admin" && role != ROLE_ADMIN {
        return Err(actix_web::error::ErrorUnauthorized("tenant not found"));
    }

    if tenant.name != aud[0] {
        return Err(actix_web::error::ErrorUnauthorized(
            "invalid token: tenant name is not match ",
        ));
    }

    let viewer = Viewer {
        role,
        player_id: token.claims.sub,
        tenant_name: tenant.name,
        tenant_id: tenant.id,
    };
    Ok(viewer)
}

async fn retrieve_tenant_row_from_header(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
) -> Result<TenantRow, sqlx::Error> {
    info!("retrieve_tenant_row_from_header now");
    // check if jwt tenant name and host header's tenant name is the same
    let base_host = get_env("ISUCON_BASE_HOSTNAME", ".t.isucon.dev");

    info!("{:?}", request.headers());
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

    match sqlx::query_as("SELECT * FROM tenant WHERE name = ?")
        .bind(tenant_name)
        .fetch_one(pool.as_ref())
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
async fn retrieve_player(tenant_db: SqlitePool, id: String) -> Result<PlayerRow, sqlx::Error> {
    info!("retrieve player now");
    let row: PlayerRow = match sqlx::query_as("SELECT * FROM player WHERE id = ?")
        .bind(id)
        .fetch_one(&tenant_db)
        .await
    {
        Ok(row) => row,
        _ => return Err(sqlx::Error::RowNotFound),
    };
    Ok(row)
}

// 参加者を認可する
// 参加者向けAPIで呼ばれる
async fn authorize_player(tenant_db: SqlitePool, id: String) -> Result<(), actix_web::Error> {
    info!("authorize player now");
    let player = match retrieve_player(tenant_db, id).await {
        Ok(player) => player,
        Err(sqlx::Error::RowNotFound) => {
            return Err(actix_web::error::ErrorUnauthorized("player not found"));
        }
        _ => panic!("error retrieve_tenant_row_from_header at parse_viewer"),
    };
    if player.is_disqualified {
        return Err(actix_web::error::ErrorForbidden("player is disqualified"));
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
    tenant_db: SqlitePool,
    id: String,
) -> Result<CompetitionRow, sqlx::Error> {
    info!("retrieve competition now");
    let row: CompetitionRow = match sqlx::query_as("SELECT * FROM competition WHERE id = ?")
        .bind(id)
        .fetch_one(&tenant_db)
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
fn lock_file_path(id: i64) -> String {
    info!("lock file path now");
    let tenant_db_dir = get_env("ISUCON_TENANT_DB_DIR", "../tenant_db");
    return Path::new(&tenant_db_dir)
        .join(format!("{}.lock", id))
        .to_str()
        .unwrap()
        .to_string();
}

// 排他ロックする
fn flock_by_tenant_id(tenant_id: i64) -> Result<i32, ()> {
    info!("flock by tennat id now");
    let p = lock_file_path(tenant_id);
    let lock_file = open(p.as_str(), OFlag::empty(), Mode::empty()).unwrap();
    match flock(lock_file, FlockArg::LockExclusiveNonblock) {
        Ok(()) => Ok(lock_file),
        Err(_) => {
            println!("existing process!");
            Err(())
        }
    }
}

#[derive(Serialize)]
struct TenantsAddHandlerResult {
    tenant: TenantWithBilling,
}

#[derive(Debug, Serialize, Deserialize)]
struct FormInfo {
    name: String,
    display_name: String,
}
// SaaS管理者用API
// テナントを追加する
// POST /api/admin/tenants/add
async fn tenants_add_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    form: web::Form<FormInfo>,
) -> actix_web::Result<HttpResponse> {
    info!("tenants_add_handler now");
    info!("before parsing");
    let v: Viewer = parse_viewer(pool.clone(), request).await?;
    info!("parse viewer ok");
    if v.tenant_name != *"admin" {
        // admin: SaaS管理者用の特別なテナント名
        return Err(actix_web::error::ErrorNotFound("you don't have this API"));
    }
    info!("tenant_name ok");
    if v.role != ROLE_ADMIN {
        return Err(actix_web::error::ErrorForbidden("admin role required"));
    }
    info!("admin role ok");
    let display_name = &form.display_name;
    let name = &form.name;
    validate_tenant_name(name.to_string()).map_err(|e| actix_web::error::ErrorBadRequest(e))?;
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("error now()")
        .as_secs() as i64;

    let insert_res = match sqlx::query(
        "INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)",
    )
    .bind(name)
    .bind(display_name)
    .bind(now)
    .bind(now)
    .execute(pool.as_ref())
    .await
    {
        Ok(insert_res) => insert_res,
        // TODO: duplicate entry  error handling
        _ => return Err(actix_web::error::ErrorBadRequest("duplicate tenant")),
    };

    let id = insert_res.last_insert_id();
    create_tenant_db(id.try_into().expect("error: try_into()")).await;
    info!("insert tenant ok");
    let res = TenantsAddHandlerResult {
        tenant: TenantWithBilling {
            id: id.to_string(),
            name: ToString::to_string(&name),
            display_name: display_name.to_string(),
            billing_yen: 0,
        },
    };
    Ok(HttpResponse::Ok().json(SuccessResult {
        status: true,
        data: Some(res),
    }))
}

// テナント名が規則に従っているかチェックする
fn validate_tenant_name(name: String) -> Result<(), String> {
    info!("validate_tenant_name now");
    if TENANT_NAME_REGEXP.is_match(name.as_str()) {
        Ok(())
    } else {
        Err(format!("invalid tenant name: {}", name))
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

#[derive(Debug, sqlx::FromRow)]
struct RowString {
    value: String,
}

// 大会ごとの課金レポートを計算する
async fn billing_report_by_competition(
    tenant_db: SqlitePool,
    tenant_id: i64,
    competition_id: String,
) -> Result<BillingReport, sqlx::Error> {
    info!("billing report by competition now");
    let comp: CompetitionRow = retrieve_competition(tenant_db.clone(), competition_id).await?;
    let vhs: Vec<VisitHistorySummaryRow> = match sqlx::query_as(
        "SELECT player_id, MIN(created_at) AS min_created_at FROM visit_history WHERE tenant_id = ? AND competition_id = ? GROUP BY player_id")
        .bind(tenant_id)
        .bind(comp.id.clone())
        .fetch_all(&tenant_db).await{
            Ok(vhs) => vhs,
            _ => return Err(sqlx::Error::RowNotFound),
        };

    let mut billing_map: HashMap<String, String> = HashMap::new();
    for vh in vhs {
        // competition.finished_atよりも後の場合は, 終了後に訪問したとみなして大会開催内アクセス済みと見做さない
        if comp.finished_at.is_some() && comp.finished_at.unwrap() < vh.min_created_at {
            continue;
        }
        billing_map.insert(vh.player_id, "visitor".to_string());
    }
    // player_scoreを読んでいる時に更新が走ると不整合が起こるのでロックを取得する
    let _fl = flock_by_tenant_id(tenant_id).unwrap();

    // スコアを登録した参加者のIDを取得する
    sqlx::query_as(
        "SELECT DISTINCT(player_id) FROM player_score WHERE tenant_id = ? AND competition_id = ?",
    )
    .bind(tenant_id)
    .bind(comp.id.clone())
    .fetch_all(&tenant_db)
    .await
    .unwrap()
    .into_iter()
    .for_each(|ps: RowString| {
        billing_map.insert(ps.value, "player".to_string());
    });

    // 大会が終了している場合のみ請求金額が確定するので計算する
    let mut player_count = 0;
    let mut visitor_count = 0;
    if comp.finished_at.is_some() {
        for (_, category) in billing_map {
            if category == *"player" {
                player_count += 1;
            } else if category == *"visitor" {
                visitor_count += 1;
            }
        }
    }
    Ok(BillingReport {
        competition_id: comp.id.clone(),
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
    billing_yen: i64,
}

#[derive(Debug, Serialize)]
struct TenantsBillingHandlerResult {
    tenants: Vec<TenantWithBilling>,
}

#[derive(serde_derive::Serialize, serde_derive::Deserialize)]
struct BillingQuery {
    before: String,
}
// SaaS管理者用API
// テナントごとの課金レポートを最大10件, テナントのid降順で取得する
// POST /api/admin/tenants/billing
// URL引数beforeを指定した場合, 指定した値よりもidが小さいテナントの課金レポートを取得する
async fn tenants_billing_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    query: web::Query<BillingQuery>,
    conn: actix_web::dev::ConnectionInfo,
) -> actix_web::Result<HttpResponse> {
    info!("tenants billing handler now");
    if conn.host() != get_env("ISUCON_ADMIN_HOSTNAME", "admin.t.isucon.dev") {
        return Err(actix_web::error::ErrorNotFound(format!(
            "invalid hostname {}",
            conn.host()
        )));
    };
    let v = parse_viewer(pool.clone(), request).await?;
    if v.role != ROLE_ADMIN {
        return Err(actix_web::error::ErrorForbidden("admin role required"));
    };
    let before = &query.before;
    let mut before_id = 0;
    if !before.is_empty() {
        before_id = if let Ok(id) = before.parse::<i64>() {
            id
        } else {
            return Err(actix_web::error::ErrorBadRequest(
                "failed to parse query parameter 'before'",
            ));
        };
    }
    // テナントごとに
    //   大会ごとに
    //     scoreに登録されているplayerでアクセスした人 * 100
    //     scoreに登録されているplayerでアクセスしていない人 * 50
    //     scoreに登録されていないplayerでアクセスした人 * 10
    //   を合計したものを
    // テナントの課金とする
    let ts: Vec<TenantRow> = sqlx::query_as("SELECT * FROM tenant ORDER BY id DESC")
        .fetch_all(pool.as_ref())
        .await
        .unwrap();
    let mut tenant_billings = Vec::<TenantWithBilling>::new();
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
        let tenant_db = connect_to_tenant_db(t.id).await.unwrap();
        let cs: Vec<CompetitionRow> = sqlx::query_as("SELECT * FROM competition WHERE tenant_id=?")
            .bind(t.id)
            .fetch_all(&tenant_db)
            .await
            .unwrap();
        for comp in cs {
            let report = billing_report_by_competition(tenant_db.clone(), t.id, comp.id)
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
) -> actix_web::Result<HttpResponse> {
    info!("players list handler now");
    let v: Viewer = parse_viewer(pool.clone(), request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(actix_web::error::ErrorForbidden("role organizer required"));
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    info!("connected tenant_db");
    let pls: Vec<PlayerRow> =
        sqlx::query_as("SELECT * FROM player WHERE tenant_id=? ORDER BY created_at DESC")
            .bind(v.tenant_id)
            .fetch_all(&tenant_db)
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
) -> actix_web::Result<HttpResponse> {
    info!("players add handler now");
    let v: Viewer = parse_viewer(pool.clone(), request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(actix_web::error::ErrorForbidden("role organizer required"));
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    info!("connected tenant db");
    let display_names: std::collections::HashSet<String> = form_param
        .into_inner()
        .into_iter()
        .filter_map(|(key, val)| (key == "display_name[]").then(|| val))
        .collect();
    let mut pds = Vec::<PlayerDetail>::new();
    info!("display_names = {:?}",display_names);

    for display_name in display_names {
        let id = dispense_id(pool.clone()).await.unwrap();
        info!("dispense_id = {}",id);
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
            .execute( &tenant_db)
            .await.unwrap();

        let p = retrieve_player(tenant_db.clone(), id).await.unwrap();
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

#[derive(serde_derive::Deserialize)]
struct DisqualifiedFormQuery {
    player_id: String,
}
// テナント管理者向けAPI
// POST /api/organizer/player/:player_id/disqualified
// 参加者を失格にする
async fn player_disqualified_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    form_param: web::Form<DisqualifiedFormQuery>,
) -> actix_web::Result<HttpResponse> {
    info!("player disqualified handler nwo");
    let v: Viewer = parse_viewer(pool.clone(), request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(actix_web::error::ErrorForbidden("role organizer required"));
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    info!("connected tenant db");
    let player_id = form_param.into_inner().player_id;
    let now: i64 = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;
    sqlx::query::<Sqlite>("UPDATE player SET is_disqualified = ?, updated_at=? WHERE id = ?")
        .bind(true)
        .bind(now)
        .bind(player_id.clone())
        .execute(&tenant_db)
        .await
        .unwrap();
    let p: PlayerRow = match retrieve_player(tenant_db, player_id).await {
        Ok(p) => p,
        Err(sqlx::Error::RowNotFound) => {
            return Err(actix_web::error::ErrorNotFound("player not found"));
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
) -> actix_web::Result<HttpResponse> {
    info!("competitions add handler now");
    let v: Viewer = parse_viewer(pool.clone(), request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(actix_web::error::ErrorForbidden("role organizer required"));
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    info!("connected tenant db");
    let title = form.title.clone();
    let now: i64 = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;
    let id = dispense_id(pool.clone()).await.unwrap();
    sqlx::query::<Sqlite>("INSERT INTO competition (id, tenant_id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?")
    .bind(id.clone())
    .bind(v.tenant_id)
    .bind(title.clone())
    .bind(Option::<i64>::None)
    .bind(now)
    .bind(now)
    .execute( &tenant_db)
    .await.unwrap();
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

#[derive(Debug, serde_derive::Serialize, serde_derive::Deserialize)]
struct CompetitionFinishFormQuery {
    competition_id: String,
}
// テナント管理者向けAPI
// POST /api/organizer/competitions/:competition_id/finish
// 大会を終了する
async fn competition_finish_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    form: web::Form<CompetitionFinishFormQuery>,
) -> actix_web::Result<HttpResponse> {
    info!("competition finish handler now");
    let v: Viewer = parse_viewer(pool.clone(), request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(actix_web::error::ErrorForbidden("role organizer required"));
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    info!("connected tenant db");
    let id = form.into_inner().competition_id;
    if id.is_empty() {
        return Err(actix_web::error::ErrorBadRequest("competition_id required"));
    }

    let _ = match retrieve_competition(tenant_db.clone(), id.clone()).await {
        Ok(c) => c,
        Err(sqlx::Error::RowNotFound) => {
            return Err(actix_web::error::ErrorNotFound("competition not found"));
        }
        _ => {
            panic!("panic at retireve_competition");
        }
    };
    let now: i64 = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;

    sqlx::query::<Sqlite>("UPDATE competition SET finished_at = ?, updated_at=? WHERE id = ?")
        .bind(now)
        .bind(now)
        .bind(id)
        .execute(&tenant_db)
        .await
        .unwrap();

    let res = SuccessResult {
        status: true,
        data: Option::<CompetitionFinishFormQuery>::None, // TODO: Option::<!>::None を使いたいが..
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
    form: web::Form<CompetitionScoreHandlerFormQuery>,
    mut payload: Multipart,
) -> actix_web::Result<HttpResponse> {
    info!("compeittion score handler now");
    let v = parse_viewer(pool.clone(), request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(actix_web::error::ErrorForbidden("role organizer required"));
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    info!("connected tenant db");
    let competition_id = form.competition_id.clone();
    if competition_id.is_empty() {
        return Err(actix_web::error::ErrorBadRequest("competition_id required"));
    };
    let comp = match retrieve_competition(tenant_db.clone(), competition_id.clone()).await {
        Ok(c) => c,
        Err(sqlx::Error::RowNotFound) => {
            return Err(actix_web::error::ErrorNotFound("competition not found"));
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
    let mut filepath = String::new();
    while let Some(item) = payload.next().await {
        let mut field = item.unwrap();
        // A multipart/form-data stream has to contain `content_disposition`
        let content_disposition = field.content_disposition();

        let filename = content_disposition.get_filename().unwrap_or("temp.csv");
        filepath = format!("./tmp/{filename}");

        // File::create is blocking operation, use threadpool
        let mut f = tokio::fs::File::create(filepath.clone()).await.unwrap();
        // Field in turn is stream of *Bytes* object
        while let Some(chunk) = field.next().await {
            f.write_all_buf(&mut chunk?).await.unwrap();
        }
    }
    let mut rdr = csv::Reader::from_path(filepath).unwrap();
    let mut header = rdr.records();
    // check if header is "player_id", "score"
    if header.next().unwrap().unwrap() != vec!["player_id", "score"] {
        return Err(actix_web::error::ErrorBadRequest("invalid CSV headers"));
    };
    // DELETEしたタイミングで参照が来ると空っぽのランキングになるのでロックする
    let _fl = flock_by_tenant_id(v.tenant_id).unwrap();
    info!("flocked now");
    let mut row_num: i64 = 0;
    let mut player_score_rows = Vec::<PlayerScoreRow>::new();
    loop {
        row_num += 1;
        // get rdr next or break
        let row = match header.next() {
            Some(row) => row,
            None => break,
        };
        let row = row.unwrap();
        if row.len() != 2 {
            panic!("row must have tow columns");
        };
        let player_id: String = row.clone()[0].to_string();
        let score_str: String = row[1].to_string();
        let _ = match retrieve_player(tenant_db.clone(), player_id.clone()).await {
            Ok(c) => c,
            Err(sqlx::Error::RowNotFound) => {
                return Err(actix_web::error::ErrorNotFound("player not found"));
            }
            _ => panic!("panic at retireve_player"),
        };
        let score: i64 = match score_str.parse() {
            Ok(s) => s,
            _ => return Err(actix_web::error::ErrorBadRequest("error parse score_str")),
        };
        let id = dispense_id(pool.clone()).await.unwrap();
        let now: i64 = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs() as i64;
        player_score_rows.push(PlayerScoreRow {
            id,
            tenant_id: v.tenant_id,
            player_id: player_id.clone(),
            competition_id: competition_id.clone(),
            score,
            row_num,
            created_at: now,
            updated_at: now,
        });
    }
    sqlx::query::<Sqlite>("DELETE FROM player_score WHERE tenant_id = ? AND competition_id = ?")
        .bind(v.tenant_id)
        .bind(competition_id.clone())
        .execute(&tenant_db.clone())
        .await
        .unwrap();

    for ps in &player_score_rows {
        sqlx::query::<Sqlite>("INSERT INTO player_score (id, tenant_id, player_id, competition_id, score, row_num, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)")
        .bind(ps.id.clone())
        .bind(ps.tenant_id)
        .bind(ps.player_id.clone())
        .bind(ps.competition_id.clone())
        .bind(ps.score)
        .bind(ps.row_num)
        .bind(ps.created_at)
        .bind(ps.updated_at)
        .execute(&tenant_db.clone())
        .await.unwrap();
    }
    let res = SuccessResult {
        status: true,
        data: Some(ScoreHandlerResult {
            rows: player_score_rows.len() as i64,
        }),
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
) -> actix_web::Result<HttpResponse> {
    info!("billing handler now");
    let v: Viewer = parse_viewer(pool.clone(), request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(actix_web::error::ErrorForbidden("role organizer required"));
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    info!("connected tenant db");
    let cs: Vec<CompetitionRow> =
        sqlx::query_as("SELECT * FROM competition WHERE tenant_id = ? ORDER BY created_at DESC")
            .bind(v.tenant_id)
            .fetch_all(&tenant_db)
            .await
            .unwrap();
    let mut tbrs = Vec::<BillingReport>::new();
    for comp in cs {
        let report: BillingReport =
            billing_report_by_competition(tenant_db.clone(), v.tenant_id, comp.id)
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

#[derive(Debug, serde_derive::Serialize, serde_derive::Deserialize)]
struct PlayerHandlerQueryParam {
    player_id: String,
}
// 参加者向けAPI
// GET /api/player/player/:player_id
// 参加者の詳細情報を取得する
async fn player_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    from: web::Query<PlayerHandlerQueryParam>,
) -> actix_web::Result<HttpResponse> {
    info!("player handler now");
    let v: Viewer = parse_viewer(pool.clone(), request).await?;
    if v.role != ROLE_PLAYER {
        return Err(actix_web::error::ErrorForbidden("role player required"));
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    info!("connected tenant db");
    authorize_player(tenant_db.clone(), v.player_id)
        .await
        .unwrap();
    let player_id = from.into_inner().player_id;
    if player_id.is_empty() {
        return Err(actix_web::error::ErrorBadRequest("player_id required"));
    };
    let p = match retrieve_player(tenant_db.clone(), player_id).await {
        Ok(p) => p,
        Err(sqlx::Error::RowNotFound) => {
            return Err(actix_web::error::ErrorNotFound("player not found"));
        }
        _ => {
            panic!("panic at retrieve_player")
        }
    };
    let cs: Vec<CompetitionRow> =
        sqlx::query_as("SELECT * FROM competition ORDER BY created_at ASC")
            .fetch_all(&tenant_db)
            .await
            .unwrap();
    // player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
    let _fl = flock_by_tenant_id(v.tenant_id);
    info!("flocked now");
    let mut pss = Vec::<PlayerScoreRow>::new();
    for c in cs {
        let ps: PlayerScoreRow = sqlx::query_as(
            "SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? AND player_id = ? ORDER BY row_num DESC LIMIT 1")
            .bind(v.tenant_id)
            .bind(c.id.clone())
            .bind(p.id.clone())
            .fetch_one(&tenant_db)
            .await
            .unwrap();
        pss.push(ps);
    }
    let mut psds = Vec::<PlayerScoreDetail>::new();
    for ps in pss {
        let comp = retrieve_competition(tenant_db.clone(), ps.competition_id.clone())
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
    competition_id: String,
    rank_after: String,
}
// 参加者向けAPI
// GET /api/player/competition/:competition_id/ranking
// 大会ごとのランキングを取得する
async fn competition_ranking_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
    form: web::Form<CompetitionRankingHandlerQueryParam>,
) -> actix_web::Result<HttpResponse> {
    info!("compeititon ranking handler now");
    let v: Viewer = parse_viewer(pool.clone(), request).await?;
    if v.role != ROLE_PLAYER {
        return Err(actix_web::error::ErrorForbidden("role player required"));
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    info!("connected tenant db now");
    let competition_id = form.clone().competition_id;
    if competition_id.is_empty() {
        return Err(actix_web::error::ErrorBadRequest("competition_id required"));
    };
    // 大会の存在確認
    let competition = match retrieve_competition(tenant_db.clone(), competition_id.clone()).await {
        Ok(c) => c,
        Err(sqlx::Error::RowNotFound) => {
            return Err(actix_web::error::ErrorNotFound("competition not found"));
        }
        _ => panic!("panic at retrieve_competiiton"),
    };
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;
    let tenant: TenantRow = sqlx::query_as("SELECT * FROM tenant WHERE id = ?")
        .bind(v.tenant_id)
        .fetch_one(&tenant_db)
        .await
        .unwrap();
    sqlx::query("INSERT INTO visit_history (player_id, tenant_id, competition_id, created_at, updated) VALUES (?, ?, ?, ?, ?)")
    .bind(v.player_id)
    .bind(tenant.id)
    .bind(competition_id.clone())
    .bind(now)
    .bind(now)
    .execute(pool.as_ref())
    .await.unwrap();
    // TODO: 数字以外の文字列を入力した場合はエラーにする

    // player_scoreを読んでいる時に更新が走ると不整合が走るのでロックを取得する
    let _fl = flock_by_tenant_id(v.tenant_id);
    info!("flocked now");
    let pss: Vec<PlayerScoreRow> = sqlx::query_as(
        "SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? ORDER BY row_num DESC")
        .bind(tenant.id)
        .bind(competition_id.clone())
        .fetch_all(&tenant_db)
        .await
        .unwrap();
    let mut ranks = Vec::<CompetitionRank>::new();
    let mut scored_player_set = HashMap::<String, bool>::new();

    for ps in pss {
        // player_scoreが同一player_id内ではrow_numの降順でソートされているので
        // 現れたのが2回目以降のplayer_idはより大きいrow_numでスコアが出ているとみなせる
        if scored_player_set.contains_key(&ps.player_id) {
            continue;
        }
        scored_player_set.insert(ps.player_id.clone(), true);
        let p = retrieve_player(tenant_db.clone(), ps.player_id.clone())
            .await
            .unwrap();
        ranks.push(CompetitionRank {
            rank: 0,
            score: ps.score,
            player_id: p.id.clone(),
            player_display_name: p.display_name.clone(),
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
    let mut paged_ranks = Vec::<CompetitionRank>::new();
    for (i, rank) in ranks.iter().enumerate() {
        let i = i as i64;
        if i < Arc::new(form.rank_after.clone()).parse::<i64>().unwrap() {
            continue;
        }
        paged_ranks.push(CompetitionRank {
            rank: i + 1,
            score: rank.score,
            player_id: rank.player_id.clone(),
            player_display_name: rank.player_display_name.clone(),
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
                id: competition.id.clone(),
                title: competition.title.clone(),
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
) -> actix_web::Result<actix_web::HttpResponse> {
    info!("player compeititons handler now");
    let v: Viewer = parse_viewer(pool.clone(), request).await?;
    if v.role != ROLE_PLAYER {
        return Err(actix_web::error::ErrorForbidden("role player required"));
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    info!("connected tenant db now");
    authorize_player(tenant_db.clone(), v.player_id.clone()).await?;
    return competitions_handler(Some(v), tenant_db.clone()).await;
}

// 主催者向けAPI
// GET /api/organizer/competitions
// 大会一覧を取得する
async fn organizer_competitions_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: HttpRequest,
) -> actix_web::Result<actix_web::HttpResponse> {
    info!("organizer competitions handler now");
    let v: Viewer = parse_viewer(pool.clone(), request).await?;
    if v.role != ROLE_ORGANIZER {
        return Err(actix_web::error::ErrorForbidden("role organizer required"));
    };
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    info!("connected tenant db now");
    return competitions_handler(Some(v), tenant_db.clone()).await;
}

async fn competitions_handler(
    v: Option<Viewer>,
    tenant_db: SqlitePool,
) -> actix_web::Result<actix_web::HttpResponse> {
    info!("competiitons handler now");
    let cs: Vec<CompetitionRow> =
        sqlx::query_as("SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC")
            .bind(v.map(|v| v.tenant_id).unwrap())
            .fetch_all(&tenant_db)
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
) -> actix_web::Result<HttpResponse> {
    info!("me handler now");
    let tenant: TenantRow =
        match retrieve_tenant_row_from_header(pool.clone(), request.clone()).await {
            Ok(t) => t,
            _ => {
                info!("{:?}", request);
                panic!("retrieve_tenant_row_from_header")
            }
        };
    let td = TenantDetail {
        name: tenant.name,
        display_name: tenant.display_name,
    };
    let v: Viewer = match parse_viewer(pool.clone(), request).await {
        Ok(v) => v,
        Err(e) if e.as_response_error().status_code().as_str() == "401" => {
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
    let tenant_db = connect_to_tenant_db(v.tenant_id).await.unwrap();
    info!("connected tenant db now");
    let p = match retrieve_player(tenant_db.clone(), v.player_id.clone()).await {
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
    info!("initialize handler now");
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
