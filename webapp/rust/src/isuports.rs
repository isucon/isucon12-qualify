use sqlx::{ConnectOptions, Connection};
use std::any::Any;
use std::path::{Path, PathBuf};
use std::str::FromStr;
use actix_web::{web, HttpResponse, Error};
use lazy_static::lazy_static;
use nix::fcntl::{flock, open, FlockArg, OFlag};
use regex::Regex;
use serde::{Deserialize, Serialize};
use sqlx::mysql::MySqlConnectOptions;
use sqlx::sqlite::SqliteConnectOptions;

const TENANT_DB_SCHEMA_FILE_PATH: &str = "../sql/tenant/10_schema.sql";
const INITIALIZE_SCRIPT: &str = "..sql/init.sh";
const COOKIE_NAME: &str = "isuports_session";

const ROLE_ADMIN: &str = "admin";
const ROLE_ORGANIZER: &str = "organizer";
const ROLE_PLAYER: &str = "player";
const ROLE_NONE: &str = "none";

lazy_static! {
    // 正しいテナント名の正規表現
    static ref TENANT_NAME_REGEXP: Regex = Regex::new("^[a-z][a-z0-9-]{0,61}[a-z0-9]$").unwrap();
    // TODO: static ref ADMIN_DB:

    static ref SQLITE_DRIBER_NAME: String = "sqlite3".to_string();
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
    return Path::new(&tenant_db_dir).join(format!("{}.db", id));
}

// テナントDBに接続する
async fn connect_to_tenant_db(id: i64)->Result<sqlx::SqliteConnection, > {
    let p = tenant_db_path(id);
    let uri = format!("sqlite:{}?mode=rw", p.to_str().unwrap());
    let conn = sqlx::SqliteConnection::connect(&uri).await?;
    Ok(conn)
    // TODO: sqliteDriverNameを使ってないのをなおす
}
// テナントDBを新規に作成する
async fn create_tenant_db(id: i64) {
    let p = tenant_db_path(id);
    let out = tokio::process::Command::new("sh").arg("-c").arg(format!(
        "sqlite3 {} < {}",
        p.to_str().unwrap(),
        TENANT_DB_SCHEMA_FILE_PATH
    ))
    .output()
    .await
    .expect(
        format!(
            "failed to exec sqlite3 {} < {}",
            p.to_str().unwrap(),
            TENANT_DB_SCHEMA_FILE_PATH
        )
        .as_str(),
    );
}

// システム全体で一意なIDを生成する
async fn dispense_id(
    pool: web::Data<sqlx::MySqlPool>,
)-> Result<String, sqlx::Error> {

    for _ in 1..100{
        sqlx::query("REPLACE INTO id_generator (stub) VALUES (?);")
            .bind("a")
            .execute(pool.as_ref())
            .await;

    }
    let id = ret.last_insert_id();
    if id != 0{
        Ok(id.to_string())
    }else{
        sqlx::Error::RowNotFound
    }
}



#[actix_web::main]
async fn run() -> std::io::Result<()> {
    // sqliteのクエリログを出力する設定
    // 環境変数 ISUCON_SQLITE_TRACE_FILEを設定すると, そのファイルにクエリログをJSON形式で出力する
    // 未設定なら出力しない
    // sqltrace.rsを参照

    let mysql_config =     MySqlConnectOptions::new()
        .host(&get_env("ISUCON_DB_HOST","127.0.0.1"))
        .username(&get_env("ISUCON_DB_USER", "isucon"))
        .password(&get_env("ISUCON_DB_PASSWORD", "isucon"))
        .database(&get_env("ISUCON_DB_NAME", "isuports"))
        .port(&get_env("ISUCON_DB_PORT"), "3306");

    let pool = sqlx::mysql::MySqlPoolOptions::new()
        .max_connections(10)
        .connect_with(mysql_config)
        .await
        .expect("failed to connect mysql db");

    let server = actix_web::HttpServer::new(move || {
        let admin_api = web::scope("/admin/tenants")
            .route("/add", web::post().to(tenants_add_handler))
            .route("/billing", web::get().to(tenants_billing_handler));
        let organizer_api = web::scope("/organizer")
            .route("players", web::get().to(players_list_handler))
            .route("players/add", web::post().to(players_add_handler))
            .route("player/{player_id}",web::post().to(player_disqualified_handler))
            .route("competitions/add",web::post().to(competitions_add_handler))
            .route("competition/{competition_id}/finish",web::post().to(competition_finish_handler))
            .route("competition/{competition_id}/score", web::post().to(competition_score_handler))
            .route("billing",web::get().to(billing_handler))
            .route("competitions", web::get().to(organizer_competitions_handler));
        let player_api = web::scope("/player")
            .route("/player/{player_id}",web::get().to(player_handler))
            .route("/competition/{competition_id}/ranking", web::get().to(competition_ranking_handler))
            .route("competitions",web::get().to(player_competitions_handler));

        actix_web::App::new()
            .app_data(web::Data::new(pool.clone()))
            .route("/initialize", web::post().to(initialize_handler))
            .service(
                web::scope("/api")
                    .service(admin_api)
                    .service(organizer_api)
                    .service(player_api)
                    .route("/me", web::get().to(me_handler))
            )
    });
    server.bind((
        "0.0.0.0",
        std::env::var("SERVER_APP_PORT")
            .ok()
            .and_then(|port_str| port_str.parse().ok())
            .unwrap_or(3000),
        ))?
        .run().await
}

// エラー処理関数
// TODO:


#[derive(Debug, Serialize)]
struct SuccessResult {
    success: bool,
    data: dyn Any,
}

#[derive(Debug, Serialize)]
struct FailureResult {
    success: bool,
    message: String,
}


#[derive(Debug, Serialize, Deserialize)]
struct Viewer {
    role: String,
    player_id: String,
    tennant_name: String,
    tennant_id: i64,
}

// リクエストヘッダをパースしてViewerを返す
fn parse_viewer() {
    // TODO:
}

// parse request header and return Viewer
async fn post_authentication(
    request: actix_web::HttpRequest,
    session: actix_session::Session,
) -> Viewer{
    let req_jwt = request
        .headers()
        .get(COOKIE_NAME)
        .map(|value| value.to_str().unwrap_or_default())
        .unwrap_or_default();

    let key_file_name = get_env("ISUCON_JWT_KEY_FILE", "./public.pem");
    let key_src = fs::read_to_string(key_file_name)
        .expect("Something went wrong reading the file");
    let key = jsonwebtoken::DecodingKey::from_rsa_pem(key_src.as_bytes());


    let token = jsonwebtoken::decode<Viewer>(req_jwt, &key, &jsonwebtoken::Validation::ES256).unwrap();
    let token = match jsonwebtoken::decode(req_jwt, &key, &jsonwebtoken::Validation::ES256) {
        Ok(token) => token,
        Err(e) => {
            if matches!(e.kind(), jsonwebtoken::errors::ErrorKind::Json(_)) {
                return Err(actix_web::error::ErrorBadRequest("invalid JWT payload"));
            } else {
                return Err(actix_web::error::ErrorForbidden("forbidden"));
            }
        }
    };
    let tr = token.claims.role;
    match tr{
        ROLE_ADMIN => tr.to_string(),
        ROLE_ORGANIZER => tr.to_string(),
        ROLE_PLAYER => tr.to_string(),
        _ => panic!("invalid role"),
    }
    // aud contains one tenant name
    let aud = token.claims.aud;
    if len(aut) != 1{
        panic!("invalid audience");
    }
    let tenant = retrieve_tenant_row_from_header();

    if tenant.name == "admin" && role != ROLE_ADMIN {
        panic!("invalid role");
    }

    if tenant.name != aud[0]{
        panic!("invalid audience");
    }

    let viewer = Viewer {
        role: tr,
        player_id: token.claims.player_id,
        tennant_name: tenant.name,
        tennant_id: tenant.id,
    };
    Viewer
}


async fn retrieve_tenant_row_from_header(
    
) {
    // TODO:

}

#[derive(Debug, Serialize)]
struct TenantRow {
    id: i64,
    name: String,
    display_name: String,
    created_at: i64,
    updatede_at: i64,
}

trait DbOrTx {
    // TODO:
}

#[derive(Debug, Serialize)]
struct PlayerRow {
    tenant_id: i64,
    id: String,
    display_name: String,
    is_disqualified: bool,
    created_at: i64,
    updated_at: i64,
}

// 参加者を取得する
fn retrieve_player() {
    // TODO:
}

// 参加者を認可する
// 参加者向けAPIで呼ばれる
fn authorize_player() {
    // TODO:
}

#[derive(Debug, Serialize)]
struct CompetitionRow {
    tenant_id: i64,
    id: String,
    title: String,
    finished_at: i64, // TODO: NullInt64にする
    created_at: i64,
    updated_at: i64,
}

// 大会を取得する
fn retrieve_competition() {
    // TODO:
}

#[derive(Debug, Serialize)]
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
    let tenant_db_dir = get_env("ISUCON_TENANT_DB_DIR", "../tenant_db");
    return Path::new(&tenant_db_dir).join(format!("{}.lock", id)).to_str().unwrap().to_string();
}

// 排他ロックする
fn flock_by_tenant_id(tenant_id: i64) -> Result<()> {
    let p = lock_file_path(tenant_id);
    let mut lock_file = open(p.as_str(), OFlag::empty(), Mode::empty())?;
    match flock(lock_file, FlockArg::LockExclusiveNonblock) {
        Ok(()) => Ok(lock_file),
        Err(e) => {
            println!("existing process!");
            Err(e)
        }
    }
}

#[derive(Debug, Serialize)]
struct TenantDetail {
    name: String,
    display_name: String,
}

#[derive(Serialize)]
struct TenantsAddHandlerResult {
    tenant: TenantDetail,
}

// SaaS管理者用API
// テナントを追加する
// POST /api/admin/tenants/add
async fn tenants_add_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}

// テナント名が規則に従っているかチェックする
fn validate_tenant_name(name: String) -> Result<()> {
    if TENANT_NAME_REGEXP.is_match(name.as_str()) {
        Ok(())
    } else {
        Err(format!("invalid tenant name: {}",name))
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

#[derive(Debug)]
struct VisitHistoryRow {
    player_id: String,
    tenant_id: i64,
    competition_id: String,
    created_at: i64,
    updated_at: i64,
}

#[derive(Debug)]
struct VisitHistorySummaryRow {
    player_id: String,
    min_created_at: i64,
}

// 大会ごとの課金レポートを計算する
fn billing_report_by_competition(
    tenant_db: &dyn DbOrTx,
    tenant_id: i64,
    competition_id: String,
) -> Result<BillingReport> {
    // TODO:
}

#[derive(Debug, Serialize)]
struct TenantWithBilling {
    id: String,
    name: String,
    display_name: String,
    billing: i64,
}

#[derive(Debug, Serialize)]
struct TenantsBillingHandlerResult {
    tenants: Vec<TenantWithBilling>,
}

// SaaS管理者用API
// テナントごとの課金レポートを最大10件, テナントのid降順で取得する
// POST /api/admin/tenants/billing
// URL引数beforeを指定した場合, 指定した値よりもidが小さいテナントの課金レポートを取得する
async fn tenants_billing_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {

    // TODO:
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
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}

// テナント管理者向けAPI
// GET /api/organizer/players/add
// テナントに参加者を追加する
async fn players_add_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}

#[derive(Debug, Serialize)]
struct PlayerDisqualifiedHandlerResult {
    player: PlayerDetail,
}

// テナント管理者向けAPI
// POST /api/organizer/player/:player_id/disqualified
// 参加者を失格にする
async fn player_disqualified_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
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

// テナント管理者向けAPI
// POST /api/organizer/competitions/add
// 大会を追加する
async fn competitions_add_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}

// テナント管理者向けAPI
// POST /api/organizer/competitions/:competition_id/finish
// 大会を終了する
async fn competition_finish_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}

#[derive(Debug, Serialize)]
struct ScoreHandlerResult {
    rows: i64,
}

// テナント管理者向けAPI
// POST /api/organizer/competitions/:competition_id/score
// 大会のスコアをCSVでアップロードする
async fn competition_score_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}

#[derive(Debug, Serialize)]
struct BillingHandlerResult {
    reports: Vec<BillingReport>,
}

// テナント管理者向けAPI
// GET /api/organizer/billing
// テナント内の課題レポートを取得する
async fn billing_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}

#[derive(Debug, Serialize)]
struct PlayerScoreDetail {
    competition_title: String,
    score: i64,
}

#[derive(Debug, Serialize)]
struct PlayerScoreHandlerResult {
    player: PlayerDetail,
    scores: Vec<PlayerScoreDetail>,
}

// 参加者向けAPI
// GET /api/player/player/:player_id
// 参加者の詳細情報を取得する
async fn player_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
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

// 参加者向けAPI
// GET /api/player/competition/:competition_id/ranking
// 大会ごとのランキングを取得する
async fn competition_ranking_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}

#[derive(Debug, Serialize)]
struct CompetitionsHandlerResult {
    competitions: Vec<CompetitionDetail>,
}

// 参加者向けAPI
// GET /api/player/competitions
// 大会一覧を取得する
async fn player_competitions_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}

// 主催者向けAPI
// GET /api/organizer/competitions
// 大会一覧を取得する
async fn organizer_competitions_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}

fn competitions_handler(v: Option<Viewer>, tenant_db: &dyn DbOrTx) -> Result<()> {
    // TODO:
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
// JWTで認証した結果, テナントやユーザ情報を返す
async fn me_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}

#[derive(Debug, Serialize)]
struct InitializeHandlerResult {
    lang: String,
    appeal: String,
}

// ベンチマーカー向けAPI
// POST /initialize
// ベンチマーカーが起動した時に最初に呼ぶ
// データベースの初期化などが実行されるため, スキーマを変更した場合などは適宜改変すること
async fn initialize_handler() -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}
