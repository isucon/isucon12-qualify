use nix::sys::stat::Mode;
use sqlx::{ConnectOptions, Connection};
use std::any::Any;
use std::fs;
use std::os::unix::prelude::RawFd;
use std::path::{Path, PathBuf};
use actix_web::{web, HttpResponse, HttpRequest};
use lazy_static::lazy_static;
use nix::fcntl::{flock, open, FlockArg, OFlag};
use regex::Regex;
use serde::{Deserialize, Serialize};
use sqlx::mysql::MySqlConnectOptions;
use std::time::{SystemTime, UNIX_EPOCH};
use std::result::Result;
use std::collections::HashMap;
use jsonwebtoken;

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
async fn connect_to_tenant_db(id: i64)->sqlx::Result<sqlx::SqliteConnection> {
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
    let mut id: u8 = 0;
    for _ in 1..100{
        let ret = sqlx::query("REPLACE INTO id_generator (stub) VALUES (?);")
            .bind("a")
            .execute(pool.as_ref())
            .await;
        let id:u8 = ret.unwrap().last_insert_id().try_into().unwrap();

    }

    if id != 0{
        Ok(id.to_string())
    }else{
        Err(sqlx::Error::RowNotFound)
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
        .port(get_env("ISUCON_DB_PORT", "3306").parse::<u16>().unwrap());

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

#[derive(Debug, Serialize,Deserialize)]
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



// parse request header and return Viewer
async fn parse_viewer(
    pool: web::Data<sqlx::MySqlPool>,
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


    let token = jsonwebtoken::decode(req_jwt, &key.unwrap(), &jsonwebtoken::Validation::ES256).unwrap();
    let token = match jsonwebtoken::decode(req_jwt, &key.unwrap(), &jsonwebtoken::Validation::ES256) {
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
    if aud.len() != 1{
        panic!("invalid audience");
    }
    let tenant = retrieve_tenant_row_from_header(pool, request);

    if tenant.name == "admin" && tr != ROLE_ADMIN {
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
    viewer
}


async fn retrieve_tenant_row_from_header(
    pool: web::Data<sqlx::MySqlPool>,
    request: actix_web::HttpRequest,
) -> Option<TenantRow> {
    // check if jwt tenant name and host header's tenant name is the same
    let base_host = get_env("ISUCON_BASE_HOSTNAME", ".t.isucon.dev");
    let tenant_name = request.headers().get("Host").unwrap().to_str().unwrap().trim_start();

    // SaaS管理者用ドメイン
    if tenant_name == "admin"{
        return Some(TenantRow{
            name: "admin".to_string(),
            display_name: "admin".to_string(),
            id: 0,
            created_at: 0,
            updated_at: 0
        })
    }

    sqlx::query(
        "SELECT * FROM tenants WHERE name = ?")
    .bind(tenant_name)
    .fetch_one(&pool).await?;

    None

}

#[derive(Debug, Serialize, Deserialize)]
struct TenantRow {
    id: i64,
    name: String,
    display_name: String,
    created_at: i64,
    updated_at: i64,
}

trait dbOrTx {
    // TODO:

}

#[derive(Debug, Serialize,Deserialize)]
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
    tenant_db: web::Data<sqlx::SqliteConnection>,
    id: String
) -> Result<PlayerRow, actix_web::Error> {
    sqlx::query(
        "SELECT * FROM player WHERE id = ?")
    .bind(id)
    .fetch_one(tenant_db).await?;
}

// 参加者を認可する
// 参加者向けAPIで呼ばれる
async fn authorize_player(
    tenant_db: web::Data<sqlx::SqliteConnection>,
    id: String,
) -> Result<(), actix_web::Error> {
    let player = retrieve_player(tenant_db, id);
    if player.is_disqualified {
        return  Err(actix_web::error::ErrorBadRequest("player is disqualified"));
    }
    Ok(())
}


#[derive(Debug, Serialize,Deserialize)]
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
    tenant_db: web::Data<sqlx::SqliteConnection>,
    id: String
) -> Result<CompetitionRow, actix_web::Error> {
    sqlx::query(
        "SELECT * FROM competition WHERE id = ?")
    .bind(id)
    .fetch_one(tenant_db).await?
}


#[derive(Debug, Serialize, Deserialize)]
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
fn flock_by_tenant_id(tenant_id: i64) -> Result<RawFd,()> {
    let p = lock_file_path(tenant_id);
    let mut lock_file = open(p.as_str(), OFlag::empty(), Mode::empty());
    match flock(lock_file, FlockArg::LockExclusiveNonblock) {
        Ok(()) => Ok(lock_file),
        Err(_) => {
            println!("existing process!");
            Err()
        }
    }
}

#[derive(Debug, Serialize,Deserialize)]
struct TenantDetail {
    name: String,
    display_name: String,
}

#[derive(Serialize)]
struct TenantsAddHandlerResult {
    tenant: TenantDetail,
}

#[derive(Debug,Serialize, Deserialize)]
struct FormInfo{
    name: String,
    display_name: String,
}
// SaaS管理者用API
// テナントを追加する
// POST /api/admin/tenants/add
async fn tenants_add_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: actix_web::HttpRequest,
    session: actix_session::Session,
    form: web::Form<FormInfo>,
) -> actix_web::Result<impl actix_web::Responder> {
    let v = parse_viewer(pool, request, session);
    if v.tenant_name != "admin"{
        // admin: SaaS管理者用の特別なテナント名
        return Err(actix_web::error::ErrorUnauthorized("you dont have this API"));
    }
    if v.role != ROLE_ADMIN {
        return Err(actix_web::error::ErrorUnauthorized("admin role required"));
    }
    let display_name = form.display_name;
    let name = form.name;
    validate_tenant_name(name.as_str())?;
    let now = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs();

    let insert_res = sqlx::query(
        "INSERT INTO tenants (name, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)")
    .bind(name)
    .bind(display_name)
    .bind(now)
    .bind(now)
    .execute(&pool).await?;
    let id = insert_res.last_insert_id();
    create_tenant_db(id)?;
    let res = TenantsAddHandlerResult {
        tenant: TenantDetail {
            name: name,
            display_name: display_name,
        },
    };
    Ok(web::Json(res))
}

// テナント名が規則に従っているかチェックする
fn validate_tenant_name(name: String) -> Result<(),String> {
    if TENANT_NAME_REGEXP.is_match(name.as_str()) {
        Ok(())
    } else {
        Err(format!("invalid tenant name: {}",name))
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

#[derive(Debug, Serialize, Deserialize)]
struct VisitHistoryRow {
    player_id: String,
    tenant_id: i64,
    competition_id: String,
    created_at: i64,
    updated_at: i64,
}

#[derive(Debug, Serialize, Deserialize)]
struct VisitHistorySummaryRow {
    player_id: String,
    min_created_at: i64,
}

// 大会ごとの課金レポートを計算する
async fn billing_report_by_competition(
    pool: web::Data<sqlx::MySqlPool>,
    tenant_db: &dyn dbOrTx,
    tenant_id: i64,
    competition_id: String,
) -> Result<BillingReport, String> {
    // TODO:
    let comp: CompetitionRow = retrieve_competition(tenant_db, competition_id)?;
    let vhs = sqlx::query(
        "SELECT player_id, MIN(created_at) AS min_created_at FROM visit_history WHERE tenant_id = ? AND competition_id = ? GROUP BY player_id")
        .bind(tenant_id)
        .bind(comp.id)
        .fetch_all(tenant_db).await?;
    let mut billing_map: HashMap<String, String> = HashMap::new();
    for vh in  vhs{
        // competition.finished_atよりも後の場合は, 終了後に訪問したとみなして大会開催内アクセス済みと見做さない
        if comp.finished_at.is_some() && comp.finished_at.unwrap() < vh.min_created_at {
            continue;
        }
        billing_map.insert(vh.player_id, "visitor".to_string());
    }
    // player_scoreを読んでいる時に更新が走ると不整合が起こるのでロックを取得する
    let fl = flock_by_tenant_id(tenant_id)?;

    // スコアを登録した参加者のIDを取得する
    sqlx::query(
        "SELECT DISTINCT(player_id) FROM player_score WHERE tenant_id = ? AND competition_id = ?")

        .bind(tenant_id)
        .bind(comp.id)
        .fetch_all(tenant_db).await?
        .into_iter()
        .for_each(|ps| {
            billing_map.insert(ps, "player".to_string());
        });

    // 大会が終了している場合のみ請求金額が確定するので計算する
    let mut player_count = 0;
    let mut visitor_count = 0;
    if comp.finished_at.is_sum(){
        for category in billing_map{
            if(category == "player"){
                player_count+=1;
            }else if(category == "visitor"){
                visitor_count+=1;
            }
        }
    }
    Ok(BillingReport {
        competition_id: comp.id,
        competition_title: comp.title,
        player_count: player_count,
        visitor_count: visitor_count,
        billing_player_yen: 100 * player_count,// スコアを登録した参加者は100円
        billing_visitor_yen: 10 * visitor_count, // ランキングを閲覧だけした(スコアを登録していない)参加者は10円
        billing_yen: 100 * player_count + 10 * visitor_count,
    })
}

#[derive(Debug, Serialize, Deserialize)]
struct TenantWithBilling {
    id: String,
    name: String,
    display_name: String,
    billing: i64,
}

#[derive(Debug, Serialize, Deserialize)]
struct TenantsBillingHandlerResult {
    tenants: Vec<TenantWithBilling>,
}

#[derive(Serialize, Deserialize)]
struct BillingQuery{
    before: String
}
// SaaS管理者用API
// テナントごとの課金レポートを最大10件, テナントのid降順で取得する
// POST /api/admin/tenants/billing
// URL引数beforeを指定した場合, 指定した値よりもidが小さいテナントの課金レポートを取得する
async fn tenants_billing_handler(
    pool: web::Data<sqlx::MySqlPool>,
    session: actix_session::Session,
    request: web::Data<HttpRequest>,
    query: web::Query<BillingQuery>,
) -> actix_web::Result<actix_web::HttpResponse> {
    let host = request.host().unwrap();
    if host != get_env("ISUCON_ADMIN_HOSTNAME", "admin.t.isucon.dev"){
        Ok(actix_web::HttpResponse::NotFound().finish())
    }
    let v = parse_viewer(pool, request, session).await?;
    if v.role != ROLE_ADMIN{
        Ok(actix_web::HttpResponse::Forbidden().finish())
    }
    let before = query.before;
    let before_id = 0;
    if before != ""{
        before_id = match i64::from_str_radix(&before, 10){
            Ok(id)=> id,
            Err(_)=> return Ok(actix_web::HttpResponse::BadRequest().body("before is not number")),
        };
    }
    // テナントごとに
	//   大会ごとに
	//     scoreに登録されているplayerでアクセスした人 * 100
	//     scoreに登録されているplayerでアクセスしていない人 * 50
	//     scoreに登録されていないplayerでアクセスした人 * 10
	//   を合計したものを
	// テナントの課金とする
    let ts: TenantRow = sqlx::query(
        "SELECT * FROM tenant ORDER BY id DESC")
        .fetch_all(pool)
        .await?;
    let tenant_billings = Vec::<TenantWithBilling>::new();
    for t in ts{
        if before_id != 0 && before_id <= t.id{
            continue;
        }
        let tb = TenantWithBilling{
            id: i64::from_str_radix(t.id, 10),
            name: t.name,
            display_name: t.display_name,
            billing: 0,
        };
        let tenant_db = connect_to_tenant_db(t.id)?;
        let cs: Vec<CompetitionRow>=sqlx::query("SELECT * FROM competition WHERE tenant_id=?")
            .bind(t.id)
            .fetch_all(tenant_db)
            .await?;
        for comp in cs{
            let report = billing_report_by_competition(pool, tenant_db, t.id, comp.id).await?;
            tb.billing_yen += report.billing_yen;
        }
        tenant_billings.push(tb);
        if tenant_billings.len() >= 10{
            break;
        }
    }
    Ok(web::Json(TenantsBillingHandlerResult {
        tenants: tenant_billings,
    }))


}

#[derive(Debug, Serialize, Deserialize)]
struct PlayerDetail {
    id: String,
    display_name: String,
    is_disqualified: bool,
}

#[derive(Debug, Serialize,Deserialize)]
struct PlayersListHandlerResult {
    players: Vec<PlayerDetail>,
}

// テナント管理者向けAPI
// GET /api/organizer/players
// 参加者一覧を返す
async fn players_list_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: web::Data<actix_web::HttpRequest>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    let v: Viewer = parse_viewer(pool, request, session).await?;
    if v.role != ROLE_ORGANIZER{
        Ok(actix_web::HttpResponse::Forbidden().finish())
    }
    let tenant_db = connect_to_tenant_db(v.tenant_id)?;
    let pls: Vec::<PlayerRow> = sqlx::query("SELECT * FROM player WHERE tenant_id=? ORDER BY created_at DESC")
        .bind(v.tenant_id)
        .fetch_all(tenant_db)
        .await?;
    let pds: Vec<PlayerDetail> = Vec::<PlayerDetail>::new();
    for p in pls{
        pds.push(PlayerDetail{
            id: p.id,
            display_name: p.display_name,
            is_disqualified: p.is_disqualified,
        });
    }
    let res = PlayersListHandlerResult{
        players: pds,
    };
    Ok(web::Json(res))
}

#[derive(Debug,Serialize,Deserialize)]
struct PlayersAddHandlerResult{
    players: Vec<PlayerDetail>,
}

#[derive(Debug,Serialize,Deserialize)]
struct PlayerAddFormQuery{
    display_name: String,
}

// テナント管理者向けAPI
// GET /api/organizer/players/add
// テナントに参加者を追加する
async fn players_add_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: web::Data<HttpRequest>,
    session: actix_session::Session,
    form_param: web::Form<PlayerAddFormQuery>,
) -> actix_web::Result<actix_web::HttpResponse> {
    let v: Viewer = parse_viewer(pool, request, session).await?;
    if v.role != ROLE_ORGANIZER{
        Ok(actix_web::HttpResponse::Forbidden().finish())
    }
    let tenant_db = connect_to_tenant_db(v.tenant_id)?;
    let display_names = form_param.into_inner().display_name;
    let pds = Vec::<PlayerDetail>::new();
    for display_name in display_names{
        let id = dispense_id(pool)?;
        let now =   SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs();
        sqlx::query("INSERT INTO player (id, tenant_id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)")
            .bind(id)
            .bind(v.tenant_id)
            .bind(display_name)
            .bind(false)
            .bind(now)
            .bind(now)
            .execute(tenant_db)
            .await?;
        let p = retrieve_player(tenant_db, id);
        pds.push(PlayerDetail{
            id: p.id,
            display_name: p.display_name,
            is_disqualified: p.is_disqualified,
        });
    }
    let res = PlayersAddHandlerResult{
            players: pds,
    };

}

#[derive(Debug, Serialize, Deserialize)]
struct PlayerDisqualifiedHandlerResult {
    player: PlayerDetail,
}

#[derive(Serialize,Deserialize)]
struct DisqualifiedFormQuery {
    player_id: String,
}
// テナント管理者向けAPI
// POST /api/organizer/player/:player_id/disqualified
// 参加者を失格にする
async fn player_disqualified_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: web::Data<HttpRequest>,
    session: actix_session::Session,
    form_param: web::Form<DisqualifiedFormQuery>,
) -> actix_web::Result<actix_web::HttpResponse> {
    let v:Viewer = parse_viewer(pool, request, session).await?;
    if v.role != ROLE_ORGANIZER{
        Ok(actix_web::HttpResponse::Forbidden().finish())
    }
    let tenant_db = connect_to_tenant_db(v.tenant_id)?;
    let player_id = form_param.into_inner().player_id;
    let now = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs();
    sqlx::query("UPDATE player SET is_disqualified = ?, updated_at=? WHERE id = ?")
        .bind(true)
        .bind(now)
        .bind(player_id)
        .execute(tenant_db)
        .await?;
    let p:PlayerRow = retrieve_player(tenant_db, player_id)?;
    let res = PlayerDisqualifiedHandlerResult{
        player: PlayerDetail{
            id: p.id,
            display_name: p.display_name,
            is_disqualified: p.is_disqualified,
        },
    };
    Ok(web::Json(res))
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

#[derive(Debug,Serialize,Deserialize)]
struct CompetitionAddHandlerFormQuery{
    title: String,
}
// テナント管理者向けAPI
// POST /api/organizer/competitions/add
// 大会を追加する
async fn competitions_add_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: web::Data<HttpRequest>,
    session: actix_session::Session,
    form: web::Form<CompetitionAddHandlerFormQuery>,

) -> actix_web::Result<actix_web::HttpResponse> {
    let v:Viewer = parse_viewer(pool, request, session).await?;
    if v.role != ROLE_ORGANIZER{
        Ok(actix_web::HttpResponse::Forbidden().finish())
    }
    let tenant_db = connect_to_tenant_db(v.tenant_id)?;
    let title = form.into_inner().title;
    let now = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs();
    let id = dispense_id(pool)?;
    sqlx::query("INSERT INTO competition (id, tenant_id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?")
    .bind(id)
    .bind(v.tenant_id)
    .bind(title)
    .bind(None)
    .bind(now)
    .bind(now)
    .execute(tenant_db)
    .await?;
    let res = CompetitionsAddHandlerResult{
        competition: CompetitionDetail{
            id: id,
            title: title,
            is_finished: false,
        },
    };
    Ok(web::Json(res))
}


#[derive(Debug, Serialize, Deserialize)]
struct CompetitionFinishFormQuery {
    competition_id: String,
}
// テナント管理者向けAPI
// POST /api/organizer/competitions/:competition_id/finish
// 大会を終了する
async fn competition_finish_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: web::Data<HttpRequest>,
    session: actix_session::Session,
    form: web::Form<CompetitionFinishFormQuery>,
) -> actix_web::Result<actix_web::HttpResponse> {
    let v:Viewer = parse_viewer(pool, request, session).await?;
    if v.role != ROLE_ORGANIZER{
        Ok(actix_web::HttpResponse::Forbidden().finish())
    }
    let tenant_db = connect_to_tenant_db(v.tenant_id).await?;
    let id = form.into_inner().competition_id;


    retrieve_competition(tenant_db, id).await?;
    let now = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs();

    sqlx::query("UPDATE competition SET finished_at = ?, updated_at=? WHERE id = ?")
        .bind(now)
        .bind(now)
        .bind(id)
        .execute(tenant_db)
        .await?;

    let res = SuccessResult{
        success: true,
        data: None
    };
    Ok(web::Json(res))
}

#[derive(Debug, Serialize, Deserialize)]
struct ScoreHandlerResult {
    rows: i64,
}

#[derive(Debug,Serialize,Deserialize)]
struct CompetitionScoreHandlerFormQuery{
    competition_id: String,
}
// テナント管理者向けAPI
// POST /api/organizer/competitions/:competition_id/score
// 大会のスコアをCSVでアップロードする
async fn competition_score_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: web::Data<HttpRequest>,
    session: actix_session::Session,
    form: web::Form<CompetitionScoreHandlerFormQuery>,
) -> actix_web::Result<actix_web::HttpResponse> {
    let v = parse_viewer(pool, request, session).await?;
    if v.role != ROLE_ORGANIZER{
        Ok(actix_web::HttpResponse::Forbidden().finish())
    }
    let tenant_db = connect_to_tenant_db(v.tenant_id)?;
    let competition_id = form.into_inner().competition_id;
    if competition_id == ""{
        return Ok(actix_web::HttpResponse::BadRequest().finish());
    }
    let comp = retrieve_competition(tenant_db, competition_id)?;
    if comp.finished_at.is_some(){
        let res = FailureResult{
            success: false,
            message: "competition is finished",
        };
        return Ok(web::Json(res));
    }
    let mut file = form.file("file").unwrap();
    let mut headers  = Vec::new();
    file.read_to_end(&mut headers).await?
}

#[derive(Debug, Serialize,Deserialize)]
struct BillingHandlerResult {
    reports: Vec<BillingReport>,
}

// テナント管理者向けAPI
// GET /api/organizer/billing
// テナント内の課題レポートを取得する
async fn billing_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: web::Data<HttpRequest>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    let v:Viewer = parse_viewer(pool, request, session).await?;
    if v.role != ROLE_ORGANIZER{
        Ok(actix_web::HttpResponse::Forbidden().finish())
    }
    let tenant_db = connect_to_tenant_db(v.tenant_id)?;

    let cs: Vec::<CompetitionRow> = sqlx::query("SELECT * FROM competition WHERE tenant_id = ? ORDER BY created_at DESC")
    .bind(v.tenant_id)
        .fetch_all(tenant_db)
        .await?;
    let tbrs = Vec::<BillingReport>::new();
    for comp in cs{
        let report:BillingReport = billing_report_by_competition(pool, tenant_db, v.tenant_id, comp.id)?;
        tbrs.push(report);
    }
    let res = SuccessResult{
        success: true,
        data: BillingHandlerResult{
            reports: tbrs,
        },
    };
    Ok(web::Json(res))
}

#[derive(Debug, Serialize,Deserialize)]
struct PlayerScoreDetail {
    competition_title: String,
    score: i64,
}

#[derive(Debug, Serialize,Deserialize)]
struct PlayerScoreHandlerResult {
    player: PlayerDetail,
    scores: Vec<PlayerScoreDetail>,
}

#[derive(Debug, Serialize, Deserialize)]
struct PlayerHandlerQueryParam {
    player_id: String,
}
// 参加者向けAPI
// GET /api/player/player/:player_id
// 参加者の詳細情報を取得する
async fn player_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: web::Data<HttpRequest>,
    session: actix_session::Session,
    from: web::Query<PlayerHandlerQueryParam>,
) -> actix_web::Result<actix_web::HttpResponse> {
    let v:Viewer = parse_viewer(pool, request, session).await?;
    if v.role != ROLE_PLAYER{
        Ok(actix_web::HttpResponse::Forbidden().finish())
    }
    let tenant_db = connect_to_tenant_db(v.tenant_id)?;
    authorize_player(tenant_db, v.player_id)?;
    let player_id = from.into_inner().player_id;
    if player_id == ""{
        return Ok(actix_web::HttpResponse::BadRequest().finish());
    }
    let p = retrieve_player(tenant_db, player_id)?;
    let cs: Vec::<CompetitionRow> = sqlx::query("SELECT * FROM competition ORDER BY created_at ASC")
    .fetch_all(tenant_db)
    .await?;
    // player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
    // TODO:

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


#[derive(Debug, Serialize, Deserialize)]
struct CompetitionRankingHandlerQueryParam {
    competition_id: String,
    rank_after: String,
}
// 参加者向けAPI
// GET /api/player/competition/:competition_id/ranking
// 大会ごとのランキングを取得する
async fn competition_ranking_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: web::Data<HttpRequest>,
    session: actix_session::Session,
    form: web::Form<CompetitionRankingHandlerQueryParam>,
) -> actix_web::Result<actix_web::HttpResponse> {
    let v:Viewer = parse_viewer(pool, request, session).await?;
    if v.role != ROLE_PLAYER{
        Ok(actix_web::HttpResponse::Forbidden().finish())
    }
    let tenant_db = connect_to_tenant_db(v.tenant_id)?;
    let competition_id = form.into_inner().competition_id;
    if competition_id == ""{
        return Ok(actix_web::HttpResponse::BadRequest().finish());
    }
    // 大会の存在確認
    let competition = retrieve_competition(tenant_db, competition_id)?;
    let now = SystemTime::now().duration_since(UNIX_EPOCH).unwrap().as_secs();
    let tenant: TenantRow = sqlx::query("SELECT * FROM tenant WHERE id = ?")
        .bind(v.tenant_id)
        .fetch_one(tenant_db)
        .await?;
    sqlx::query("INSERT INTO visit_history (player_id, tenant_id, competition_id, created_at, updated) VALUES (?, ?, ?, ?, ?)")
    .bind(v.player_id)
    .bind(tenant.id)
    .bind(competition_id)
    .bind(now)
    .bind(now)
    .execute(pool)
    .await?;
    let rank_after_str = form.into_inner().rank_after;
    if rank_after_str != ""{
        let  rank_after = i64::from_str_radix(&rank_after_str, 10)?;
    }

    // player_scoreを読んでいる時に更新が走ると不整合が走るのでロックを取得する
    // TODO:
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
    request: web::Data<HttpRequest>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    let v:Viewer = parse_viewer(pool, request, session).await?;
    if v.role != ROLE_PLAYER{
        Ok(actix_web::HttpResponse::Forbidden().finish())
    }
    let tenant_db = connect_to_tenant_db(v.tenant_id)?;
    authorize_player(tenant_db, v.player_id)?;
    return competitions_handler(Ok(v), tenant_db).await;
}

// 主催者向けAPI
// GET /api/organizer/competitions
// 大会一覧を取得する
async fn organizer_competitions_handler(
    pool: web::Data<sqlx::MySqlPool>,
    request: web::Data<HttpRequest>,
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    let v:Viewer = parse_viewer(pool, request, session).await?;
    if v.role != ROLE_ORGANIZER{
        Ok(actix_web::HttpResponse::Forbidden().finish())
    }
    let tenant_db = connect_to_tenant_db(v.tennant_id)atait?;
    return competitions_handler(Some(v), tenant_db).await;
}

async fn competitions_handler(v: Option<Viewer>, tenant_db: &dyn dbOrTx) -> actix_web::Result<actix_web::HttpResponse> {
    let cs: Vec::<CompetitionRow> = sqlx::query("SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC")
    .bind(v.map(|v| v.tennant_id).unwrap_or(""))
    .fetch_all(tenant_db)
    .await?;
    let cds = Vec::<CompetitionDetail>::new();
    for comp in cs{
        cds.append(CompetitionDetail{
            id: comp.id,
            title: comp.title,
            is_finished: comp.finished_at.is_some(),
        })
    }
    let res = SuccessResult{
        success: true,
        data: CompetitionsHandlerResult{
            competitions: cds,
        }
    };
    Ok(HttpResponse::Ok().json(res))
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
    session: actix_session::Session,
) -> actix_web::Result<actix_web::HttpResponse> {
    // TODO:
}

#[derive(Debug, Serialize, Deserialize)]
struct InitializeHandlerResult {
    lang: String,
    appeal: String,
}

// ベンチマーカー向けAPI
// POST /initialize
// ベンチマーカーが起動した時に最初に呼ぶ
// データベースの初期化などが実行されるため, スキーマを変更した場合などは適宜改変すること
async fn initialize_handler(pool: web::Data<sqlx::MySqlPool>) -> actix_web::Result<HttpResponse> {
    if !tokio::process::Command::new(INITIALIZE_SCRIPT).output().await.is_ok() {
        return Err(actix_web::error::ErrorInternalServerError(""));
    }
    let res = InitializeHandlerResult {
        lang: "rust".to_string(),
        // 頑張ったポイントやこだわりポイントがあれば書いてください
        // 競技中の最後に計測したものを参照して, 講評記事などで使わせていただきます
        appeal: "".to_string(),
    };

    Ok(HttpResponse::Ok().json(res))
}