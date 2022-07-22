package Isuports::Web;
use v5.36;
use utf8;
use experimental qw(builtin try isa defer);
use builtin qw(true false);

use Kossy;
use HTTP::Status qw(:constants);
use Crypt::JWT qw(decode_jwt);
use Crypt::PK::RSA;
use Fcntl qw(LOCK_EX LOCK_UN O_RDWR O_CREAT);
use Text::CSV_XS;
use DBIx::Sunny;
use Cpanel::JSON::XS;
use Cpanel::JSON::XS::Type;

# sqliteのクエリログを出力する設定
# 環境変数 ISUCON_SQLITE_TRACE_FILE を設定すると、そのファイルにクエリログをJSON形式で出力する
# 未設定なら出力しない
if ($ENV{ISUCON_SQLITE_TRACE_FILE}) {
    require Isuports::SQLiteTracer;
    our $tracer = Isuports::SQLiteTracer->new(
        file => $ENV{ISUCON_SQLITE_TRACE_FILE},
    );
}

$Kossy::JSON_SERIALIZER = Cpanel::JSON::XS->new()->ascii(0);

use constant {
    TENANT_DB_SCHEMA_FILEPATH => "../sql/tenant/10_schema.sql",
    INITIALIZE_SCRIPT         => "../sql/init.sh",
    COOKIE_NAME               => "isuports_session",
};

use constant {
    ROLE_ADMIN     => "admin",
    ROLE_ORGANIZER => "organizer",
    ROLE_PLAYER    => "player",
    ROLE_NONE      => "none",
};

# 正しいテナント名の正規表現
use constant TENANT_NAME_REGEXP => qr/^[a-z][a-z0-9-]{0,61}[a-z0-9]$/;

# 管理用DBに接続する
sub connect_admin_db() {
    my $host     = $ENV{ISUCON_DB_HOST}       || '127.0.0.1';
    my $port     = $ENV{ISUCON_DB_PORT}       || '3306';
    my $user     = $ENV{ISUCON_DB_USER}       || 'isucon';
    my $password = $ENV{ISUCON_DB_PASSWORD}   || 'isucon';
    my $dbname   = $ENV{ISUCON_DB_NAME}       || 'isuports';

    my $dsn = "dbi:mysql:database=$dbname;host=$host;port=$port";
    my $dbh = DBIx::Sunny->connect($dsn, $user, $password, {
        mysql_enable_utf8mb4 => 1,
        mysql_auto_reconnect => 1,
    });
    return $dbh;
}

# テナントDBのパスを返す
sub tenant_db_path($id) {
    my $tenant_db_dir = $ENV{ISUCON_TENANT_DB_DIR} || "../tenant_db";
    return join '/', $tenant_db_dir, sprintf('%d.db', $id);
}

# テナントDBに接続する
sub connect_to_tenant_db($id) {
    my $p = tenant_db_path($id);

    my $dsn = "dbi:SQLite:$p";
    my $dbh = DBIx::Sunny->connect($dsn, "", "", {});
    return $dbh;
}

# テナントDBを新規に作成する
sub create_tenant_db($id) {
    my $p = tenant_db_path($id);

    my $err = system("sh", "-c", sprintf("sqlite3 %s < %s", $p, TENANT_DB_SCHEMA_FILEPATH));
    if ($err) {
        return sprintf("failed to exec sqlite3 %s < %s, %s", $p, TENANT_DB_SCHEMA_FILEPATH, $err)
    }
    return;
}

sub admin_db($self) {
    $self->{dbh} ||= connect_admin_db();
}


# システム全体で一意なIDを生成する
sub dispense_id($self) {
    my ($id, $last_err);
    for (my $i = 0; $i < 100; $i++) {
        try {
            $self->admin_db->query("REPLACE INTO id_generator (stub) VALUES (?);", "a")
        }
        catch ($e) {
            if ($DBI::err == 1213) { # deadlock
                $last_err = sprintf("error REPLACE INTO id_generator: %s", $e);
                next;
            }
            die $e; #rethrow
        }

        $id = $self->admin_db->last_insert_id;
        last;
    }
    if ($id != 0) {
        return sprintf('%x', $id), undef;
    }
    return "", $last_err;
}


# SaaS管理者向けAPI
post '/api/admin/tenants/add'     => \&tenants_add_handler;
get  '/api/admin/tenants/billing' => \&tenants_billing_handler;

# テナント管理者向けAPI - 参加者追加、一覧、失格
get  '/api/organizer/players'                         => \&players_list_handler;
post '/api/organizer/players/add'                     => \&players_add_handler;
post '/api/organizer/player/{player_id}/disqualified' => \&player_disqualified_handler;

# テナント管理者向けAPI - 大会管理
post '/api/organizer/competitions/add'                    => \&competitions_add_handler;
post '/api/organizer/competition/{competition_id}/finish' => \&competition_finish_handler;
post '/api/organizer/competition/{competition_id}/score'  => \&competition_score_handler;
get  '/api/organizer/billing'                             => \&billing_handler;
get  '/api/organizer/competitions'                        => \&organizer_competitions_handler;

# 参加者向けAPI
get  '/api/player/player/{player_id}'                     => \&player_handler;
get  '/api/player/competition/{competition_id}/ranking'   => \&competition_ranking_handler;
get  '/api/player/competitions'                           => \&player_competitions_handler;

# 全ロール及び未認証でも使えるhandler
get  '/api/me' => \&me_handler;

# ベンチマーカー向けAPI
post '/initialize' => \&initialize_handler;


sub SuccessResult($json_spec=undef) {
    return {
        status => JSON_TYPE_BOOL,
        $json_spec ? (data => $json_spec) : (),
    }
}

sub FailureResult() {
    return {
        status  => JSON_TYPE_BOOL,
        message => JSON_TYPE_STRING,
    }
}

sub fail($c, $code, $message) {
    warn sprintf("error at %s: %s", $c->request->uri, $message);

    my $res = $c->render_json({
        status  => false,
        message => $message,
    }, FailureResult);

    die Kossy::Exception->new($code, response => $res);
};


# リクエストヘッダをパースしてViewerを返す
sub parse_viewer($self, $c) {
    my $token_str = $c->req->cookies->{+COOKIE_NAME};
    unless ($token_str) {
        fail($c, HTTP_UNAUTHORIZED, sprintf("cookie %s is not found", COOKIE_NAME));
    }

    my $key_file_name = $ENV{"ISUCON_JWT_KEY_FILE"} || "../public.pem";
    my $key = Crypt::PK::RSA->new($key_file_name);

    my $token;
    try {
        $token = decode_jwt(token => $token_str, key => $key, accepted_alg => 'RS256');
    }
    catch ($e) {
        fail($c, HTTP_UNAUTHORIZED, $e);
    }

    unless (exists $token->{sub}) {
        fail($c, HTTP_UNAUTHORIZED, sprintf("invalid token: subject is not found in token: %s", $token_str));
    }

    unless(exists $token->{role}) {
        fail($c, HTTP_UNAUTHORIZED, sprintf("invalid token: role is not found: %s", $token_str));
    }

    my $role = $token->{role};
    unless ($role eq ROLE_ADMIN || $role eq ROLE_ORGANIZER || $role eq ROLE_PLAYER) {
        fail($c, HTTP_UNAUTHORIZED, sprintf("invalid token: %s is invalid role: %s", $role, $token_str));
    }

    # aud は1要素でテナント名がはいっている
    my $aud = $token->{aud};
    unless ((ref $aud||'' eq 'ARRAY') && ($aud->@* == 1)) {
        fail($c, HTTP_UNAUTHORIZED, sprintf("invalid token: aud field is few or too much: %s", $token_str));
    }

    my ($tenant, $err) = $self->retrieve_tenant_row_from_header($c);
    if ($err) {
        fail($c, HTTP_UNAUTHORIZED, sprintf("tenant not found"));
    }

    if ($tenant->{name} eq 'admin' && $role ne ROLE_ADMIN) {
        fail($c, HTTP_UNAUTHORIZED, "tenant not found");
    }

    if ($tenant->{name} ne $aud->[0]) {
        fail($c, HTTP_UNAUTHORIZED, sprintf("invalid token: tenant name is not match with %s: %s", $c->request->env->{HTTP_HOST}, $token_str));
    }

    return {
        role        => $role,
        player_id   => $token->{sub},
        tenant_name => $tenant->{name},
        tenant_id   => $tenant->{id},
    };
}

sub retrieve_tenant_row_from_header($self, $c) {
    # JWTに入っているテナント名とHostヘッダのテナント名が一致しているか確認
    my $base_host = $ENV{"ISUCON_BASE_HOSTNAME"} || ".t.isucon.dev";
    my $tenant_name = $c->request->env->{HTTP_HOST} =~ s/$base_host$//r;

    # SaaS管理者用ドメイン
    if ($tenant_name eq "admin") {
        return {
            name => "admin",
            display_name => "admin",
        }, undef;
    }

    # テナントの存在確認
    my $tenant = $self->admin_db->select_row("SELECT * FROM tenant WHERE name = ?", $tenant_name);
    unless ($tenant) {
        return undef, sprintf("failed to Select tenant: name=%s", $tenant_name);
    }
    return $tenant, undef;
}

# 参加者を取得する
sub retrieve_player($self, $c, $tenant_db, $id) {
    my $player = $tenant_db->select_row("SELECT * FROM player WHERE id = ?", $id);
    unless ($player) {
        return undef, sprintf("error Select player: id=%s", $id);
    }
    return $player, undef;
}

# 参加者を認可する
# 参加者向けAPIで呼ばれる
sub authorize_player($self, $c, $tenant_db, $id) {
    my ($player, $err) = $self->retrieve_player($c, $tenant_db, $id);
    if ($err) {
        fail($c, HTTP_UNAUTHORIZED, "player not found");
    }
    if ($player->{is_disqualified}) {
        fail($c, HTTP_FORBIDDEN, "player_is disqualified");
    }
    return;
}

# 大会を取得する
sub retrieve_competition($self, $c, $tenant_db, $id) {
    my $competition = $tenant_db->select_row("SELECT * FROM competition WHERE id = ?", $id);
    unless ($competition) {
        return undef, sprintf("error Select competition: id=%s", $id);
    }
    return $competition, undef;
}

# 排他ロックのためのファイル名を生成する
sub lock_file_path($id) {
    my $tenant_db_dir = $ENV{"ISUCON_TENANT_DB_DIR"} || "../tenant_db";

    return join "/", $tenant_db_dir, sprintf("%d.lock", $id);
}

# 排他ロックする
sub flock_by_tenant_id($tenant_id) {
    my $p = lock_file_path($tenant_id);

    sysopen(my $fh, $p, O_RDWR|O_CREAT) or die sprintf("cannot open lock file: %s, %s", $p, $!);

    flock($fh, LOCK_EX) or die sprintf("error flock lock: path=%s, %s", $p, $!);

    return $fh;
}

use constant TenantWithBilling => {
    id           => JSON_TYPE_STRING,
    name         => JSON_TYPE_STRING,
    display_name => JSON_TYPE_STRING,
    billing      => JSON_TYPE_INT,
};

use constant TenantsAddHandlerSuccess => SuccessResult({
    tenant => TenantWithBilling,
});

# SasS管理者用API
# テナントを追加する
# POST /api/admin/tenants/add
sub tenants_add_handler($self, $c) {
    my $v = $self->parse_viewer($c);
    unless ($v->{tenant_name} eq 'admin') {
        # admin: SaaS管理者用の特別なテナント名
        fail($c, HTTP_NOT_FOUND, "%s has not this API", $v->{tenant_name});
    }
    unless ($v->{role} eq ROLE_ADMIN) {
        fail($c, HTTP_FORBIDDEN, "admin role required");
    }

    my $display_name = $c->request->body_parameters->{display_name};
    my $name = $c->request->body_parameters->{name};

    if (my $err = validate_tenant_name($name)) {
        fail($c, HTTP_BAD_REQUEST, $err);
    }

    my $now = time;
    try {
        $self->admin_db->query(
            "INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)",
            $name, $display_name, $now, $now,
        );
    }
    catch ($e) {
        if ($DBI::err == 1062) { # duplicate entry
            fail($c, HTTP_BAD_REQUEST, "duplicate tenant");
        }
        fail($c, HTTP_INTERNAL_SERVER_ERROR, sprintf(
            "error Insert tenant: name=%s, displayName=%s, createdAt=%d, updatedAt=%d, %s",
            $name, $display_name, $now, $now, $e,
        ));
    }

    my $id = $self->admin_db->last_insert_id;

    # NOTE: 先にadminDBに書き込まれることでこのAPIの処理中に
    #       /api/admin/tenants/billingにアクセスされるとエラーになりそう
    #       ロックなどで対処したほうが良さそう
    my $err = create_tenant_db($id);
    if ($err) {
        fail($c, HTTP_INTERNAL_SERVER_ERROR, sprintf("error createTenantDB: id=%d name=%s %w", $id, $name, $err));
    }

    return $c->render_json({
        status => true,
        data => {
            tenant => {
                id           => $id,
                name         => $name,
                display_name => $display_name,
                billing      => 0,
            }
        }
    }, TenantsAddHandlerSuccess);
}

# テナント名が規則に沿っているかチェックする
sub validate_tenant_name($name) {
    if ($name =~ TENANT_NAME_REGEXP) {
        return;
    }
    return sprintf("invalid tenant name: %s", $name)
}

use constant BillingReport => {
    competition_id      => JSON_TYPE_STRING,
    competition_title   => JSON_TYPE_STRING,
    player_count        => JSON_TYPE_INT, # スコアを登録した参加者数
    visitor_count       => JSON_TYPE_INT, # ランキングを閲覧だけした(スコアを登録していない)参加者数
    billing_player_yen  => JSON_TYPE_INT, # 請求金額 スコアを登録した参加者分
    billing_visitor_yen => JSON_TYPE_INT, # 請求金額 ランキングを閲覧だけした(スコアを登録していない)参加者分
    billing_yen         => JSON_TYPE_INT, # 合計請求金額
};

# 大会ごとの課金レポートを計算する
sub billing_report_by_competition($self, $c, $tenant_db, $tenant_id, $competiton_id) {
    my ($comp, $err) = $self->retrieve_competition($c, $tenant_db, $competiton_id);
    if ($err) {
        fail($c, HTTP_INTERNAL_SERVER_ERROR, sprintf("error retrieve_competition: %s", $err));
    }

    # ランキングにアクセスした参加者のIDを取得する
    my $visit_history_summaries = $self->admin_db->select_all(
        "SELECT player_id, MIN(created_at) AS min_created_at FROM visit_history WHERE tenant_id = ? AND competition_id = ? GROUP BY player_id",
        $tenant_id,
        $comp->{id},
    );

    my $billing_map = {};
    for my $vh ($visit_history_summaries->@*) {
        # competition.finished_atよりもあとの場合は、終了後に訪問したとみなして大会開催内アクセス済みとみなさない
        if ($comp->{finished_at} && $comp->{finished_at} < $vh->{min_created_at}) {
            next
        }
        $billing_map->{$vh->{player_id}} = "visitor";
    }

    # player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
    my $fl = flock_by_tenant_id($tenant_id);
    defer { close $fl }

    # スコアを登録した参加者のIDを取得する
    my $scored_players = $tenant_db->select_all(
        "SELECT DISTINCT(player_id) AS player_id FROM player_score WHERE tenant_id = ? AND competition_id = ?",
        $tenant_id, $comp->{id},
    );
    for my $sp ($scored_players->@*) {
        # スコアが登録されている参加者
        $billing_map->{$sp->{player_id}} = "player"
    }

    # 大会が終了している場合のみ請求金額が確定するので計算する
    my ($player_count, $visitor_count) = (0,0);
    if ($comp->{finished_at}) {
        for my $category (values $billing_map->%*) {
            if ($category eq 'player') {
                $player_count++
            }
            if ($category eq 'visitor') {
                $visitor_count++
            }
        };
    }

    # BillingReport
    return {
        competition_id      => $comp->{id},
        competition_title   => $comp->{title},
        player_count        => $player_count,
        visitor_count       => $visitor_count,
        billing_player_yen  => 100 * $player_count, # スコアを登録した参加者は100円
        billing_visitor_yen => 10 * $visitor_count, # ランキングを閲覧だけした(スコアを登録していない)参加者は10円
        billing_yen         => 100*$player_count + 10*$visitor_count,
    }
}

use constant TenantsBillingHandlerSuccess => SuccessResult({
    tenants => json_type_arrayof(TenantWithBilling),
});

# SaaS管理者用API
# テナントごとの課金レポートを最大10件、テナントのid降順で取得する
# GET /api/admin/tenants/billing
# URL引数beforeを指定した場合、指定した値よりもidが小さいテナントの課金レポートを取得する
sub tenants_billing_handler($self, $c) {
    unless ($c->request->env->{HTTP_HOST} eq ($ENV{ISUCON_ADMIN_HOSTNAME} || "admin.t.isucon.dev")) {
        fail($c, HTTP_NOT_FOUND, sprintf("invalid hostname %s", $c->request->env->{HTTP_HOST}));
    }

    my $v = $self->parse_viewer($c);
    unless ($v->{role} eq ROLE_ADMIN) {
        fail($c, HTTP_FORBIDDEN, "admin role required");
    }

    my $before_id = $c->request->query_parameters->{"before"} || 0;
    # テナントごとに
    #   大会ごとに
    #     scoreが登録されているplayer * 100
    #     scoreが登録されていないplayerでアクセスした人 * 10
    #   を合計したものを
    # テナントの課金とする
    my $tenants = $self->admin_db->select_all(
        "SELECT * FROM tenant ORDER BY id DESC"
    );

    my $tenant_billings = [];
    for my $tenant ($tenants->@*) {
        if ($before_id != 0 && $before_id <= $tenant->{id}) {
            next;
        }

        my $tenant_billing = {
            id           => $tenant->{id},
            name         => $tenant->{name},
            display_name => $tenant->{display_name},
            billing      => 0,
        };

        my $tenant_db = connect_to_tenant_db($tenant->{id});
        defer { $tenant_db->disconnect }

        my $competitions = $tenant_db->select_all(
            "SELECT * FROM competition WHERE tenant_id=?",
            $tenant->{id},
        );

        for my $comp ($competitions->@*) {
            my $report = $self->billing_report_by_competition($c, $tenant_db, $tenant->{id}, $comp->{id});

            $tenant_billing->{billing} += $report->{billing_yen};
        }

        push $tenant_billings->@* => $tenant_billing;

        if ($tenant_billings->@* >= 10) {
            last;
        }
    }

    return $c->render_json({
        status => true,
        data => {
            tenants => $tenant_billings,
        },
    }, TenantsBillingHandlerSuccess);
}

use constant PlayerDetail => {
    id              => JSON_TYPE_STRING,
    display_name    => JSON_TYPE_STRING,
    is_disqualified => JSON_TYPE_BOOL,
};

use constant PlayersListHandlerSuccess => SuccessResult({
    players => json_type_arrayof(PlayerDetail),
});


# テナント管理者向けAPI
# GET /api/organizer/players
# 参加者一覧を返す
sub players_list_handler($self, $c) {
    my $v = $self->parse_viewer($c);
    unless ($v->{role} eq ROLE_ORGANIZER) {
        fail($c, HTTP_FORBIDDEN, "role organizer required");
    }

    my $tenant_db = connect_to_tenant_db($v->{tenant_id});
    defer { $tenant_db->disconnect }

    my $players = $tenant_db->select_all(
        "SELECT * FROM player WHERE tenant_id=? ORDER BY created_at DESC",
        $v->{tenant_id},
    );

    my $player_details = [];
    for my $p ($players->@*) {
        push $player_details->@* => {
            id => $p->{id},
            display_name => $p->{display_name},
            is_disqualified => $p->{is_disqualified},
        };
    }

    return $c->render_json({
        status => true,
        data => {
            players => $player_details,
        }
    }, PlayersListHandlerSuccess);
}

use constant PlayerAddHandlerSuccess => SuccessResult({
    players => json_type_arrayof(PlayerDetail),
});

# テナント管理者向けAPI
# POST /api/organizer/players/add
# テナントに参加者を追加する
sub players_add_handler($self, $c) {
    my $v = $self->parse_viewer($c);
    unless ($v->{role} eq ROLE_ORGANIZER) {
        fail($c, HTTP_FORBIDDEN, "role organizer required");
    }

    my $tenant_db = connect_to_tenant_db($v->{tenant_id});
    defer { $tenant_db->disconnect }

    my @display_names = $c->request->body_parameters->get_all("display_name[]");

    my $player_details = [];
    for my $display_name (@display_names) {
        my ($id, $err) = $self->dispense_id();
        if ($err) {
            fail($c, HTTP_INTERNAL_SERVER_ERROR, "error dispenseID: %s", $err);
        }

        my $now = time;

        $tenant_db->query(
            "INSERT INTO player (id, tenant_id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
            $id, $v->{tenant_id}, $display_name, false, $now, $now,
        );

        (my $player, $err) = $self->retrieve_player($c, $tenant_db, $id);
        if ($err) {
            fail($c, HTTP_INTERNAL_SERVER_ERROR, sprintf("error retrieve_player: %w", $err));
        }

        push $player_details->@* => {
            id              => $player->{id},
            display_name    => $player->{display_name},
            is_disqualified => $player->{is_disqualified},
        }
    }

    return $c->render_json({
        status => true,
        data => {
            players => $player_details,
        }
    }, PlayerAddHandlerSuccess);
}

use constant PlayerDisqualifiedHandlerSuccess => SuccessResult({
    player => PlayerDetail
});

# テナント管理者向けAPI
# POST /api/organizer/player/:player_id/disqualified
# 参加者を失格にする
sub player_disqualified_handler($self, $c) {
    my $v = $self->parse_viewer($c);
    unless ($v->{role} eq ROLE_ORGANIZER) {
        fail($c, HTTP_FORBIDDEN, "role organizer required");
    }

    my $tenant_db = connect_to_tenant_db($v->{tenant_id});
    defer { $tenant_db->disconnect }

    my $player_id = $c->args->{player_id};

    my $now = time;
    $tenant_db->query(
        "UPDATE player SET is_disqualified = ?, updated_at = ? WHERE id = ?",
        true, $now, $player_id,
    );

    my ($player, $err) = $self->retrieve_player($c, $tenant_db, $player_id);
    if ($err) { # 存在しないプレイヤー
        fail($c, HTTP_NOT_FOUND, "player not found");
    }

    return $c->render_json({
        status => true,
        data => {
            player => {
                id              => $player->{id},
                display_name    => $player->{display_name},
                is_disqualified => $player->{is_disqualified},
            },
        },
    }, PlayerDisqualifiedHandlerSuccess);
}

use constant CompetitionDetail => {
    id          => JSON_TYPE_STRING,
    title       => JSON_TYPE_STRING,
    is_finished => JSON_TYPE_BOOL,
};

use constant CompetitionsAddHandlerSuccess => SuccessResult({
    competition => CompetitionDetail,
});


# テナント管理者向けAPI
# POST /api/organizer/competitions/add
# 大会を追加する
sub competitions_add_handler($self, $c) {
    my $v = $self->parse_viewer($c);
    unless ($v->{role} eq ROLE_ORGANIZER) {
        fail($c, HTTP_FORBIDDEN, "role organizer required");
    }

    my $tenant_db = connect_to_tenant_db($v->{tenant_id});
    defer { $tenant_db->disconnect }

    my $title = $c->request->body_parameters->{title};
    my $now = time;

    my ($id, $err) = $self->dispense_id();
    if ($err) {
        fail($c, HTTP_INTERNAL_SERVER_ERROR, "error dispenseID: %s", $err);
    }

    $tenant_db->query(
        "INSERT INTO competition (id, tenant_id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
        $id, $v->{tenant_id}, $title, undef, $now, $now,
    );

    return $c->render_json({
        status => true,
        data => {
            competition => {
                id => $id,
                title => $title,
                is_finished => false,
            },
        }
    }, CompetitionsAddHandlerSuccess);
}

# テナント管理者向けAPI
# POST /api/organizer/competition/:competition_id/finish
# 大会を終了する
sub competition_finish_handler($self, $c) {
    my $v = $self->parse_viewer($c);
    unless ($v->{role} eq ROLE_ORGANIZER) {
        fail($c, HTTP_FORBIDDEN, "role organizer required");
    }

    my $tenant_db = connect_to_tenant_db($v->{tenant_id});
    defer { $tenant_db->disconnect }

    my $id = $c->args->{competition_id};
    unless ($id) {
        fail($c, HTTP_BAD_REQUEST, "competition_id required")
    }

    my (undef, $err) = $self->retrieve_competition($c, $tenant_db, $id);
    if ($err) { # 存在しない大会
        fail($c, HTTP_NOT_FOUND, "competition not found");
    }

    my $now = time;

    $tenant_db->query(
        "UPDATE competition SET finished_at = ?, updated_at = ? WHERE id = ?",
        $now, $now, $id,
    );

    return $c->render_json({ status => true }, SuccessResult);
}

use constant ScoreHandlerSuccess => SuccessResult({
    rows => JSON_TYPE_INT,
});

# テナント管理者向けAPI
# POST /api/organizer/competition/:competition_id/score
# 大会のスコアをCSVでアップロードする
sub competition_score_handler($self, $c) {
    my $v = $self->parse_viewer($c);
    unless ($v->{role} eq ROLE_ORGANIZER) {
        fail($c, HTTP_FORBIDDEN, "role organizer required");
    }

    my $tenant_db = connect_to_tenant_db($v->{tenant_id});
    defer { $tenant_db->disconnect }

    my $competition_id = $c->args->{competition_id};
    unless ($competition_id) {
        fail($c, HTTP_BAD_REQUEST, "competition_id required")
    }

    my ($comp, $err) = $self->retrieve_competition($c, $tenant_db, $competition_id);
    if ($err) { # 存在しない大会
        fail($c, HTTP_NOT_FOUND, "competition not found");
    }

    if ($comp->{finished_at}) {
        my $res = $c->render_json({
            status => false,
            message => "competition is finished",
        }, FailureResult);
        $res->code(HTTP_BAD_REQUEST);
        return $res;
    }

    my $file = $c->request->uploads->{scores};
    unless ($file) {
        fail($c, HTTP_INTERNAL_SERVER_ERROR, "error uploads->{scores}");
    }
    open my $fh, '<', $file->path or fail($c, HTTP_INTERNAL_SERVER_ERROR, "error open uploads->{scores}");

    my $csv = Text::CSV_XS->new();
    my $headers = $csv->getline($fh);
    unless ($headers && $headers->@* == 2 && $headers->[0] eq 'player_id' && $headers->[1] eq 'score') {
        fail($c, HTTP_BAD_REQUEST, "invalid CSV headers");
    }

    # DELETEしたタイミングで参照が来ると空っぽのランキングになるのでロックする
    my $fl = flock_by_tenant_id($v->{tenant_id});
    defer { close $fl }

    my $row_num = 0;
    my $player_score_rows = [];
    while (my $row = $csv->getline($fh)) {
        $row_num++;
        unless ($row->@* == 2) {
            fail($c, sprintf("row must have two columns: %s", join ',', $row->@*));
        }

        my ($player_id, $score_str) = $row->@*;
        my ($player, $err) = $self->retrieve_player($c, $tenant_db, $player_id);
        if ($err) { # 存在しない参加者が含まれている
            fail($c, HTTP_BAD_REQUEST, sprintf('player not found: %s', $player_id));
        }
        my $score = $score_str+0;

        (my $id, $err) = $self->dispense_id();
        if ($err) {
            fail($c, HTTP_INTERNAL_SERVER_ERROR, "error dispenseID: %s", $err);
        }
        my $now = time;
        push $player_score_rows->@* => {
            id              => $id,
            tenant_id       => $v->{tenant_id},
            player_id       => $player_id,
            competition_id  => $competition_id,
            score           => $score,
            row_num         => $row_num,
            created_at      => $now,
            updated_at      => $now,
        };
    }

    $tenant_db->query(
        "DELETE FROM player_score WHERE tenant_id = ? AND competition_id = ?",
        $v->{tenant_id},
        $competition_id,
    );

    for my $player_score ($player_score_rows->@*) {
        $tenant_db->query(
            "INSERT INTO player_score (id, tenant_id, player_id, competition_id, score, row_num, created_at, updated_at) VALUES (:id, :tenant_id, :player_id, :competition_id, :score, :row_num, :created_at, :updated_at)",
            $player_score,
        );
    }

    return $c->render_json({
        status => true,
        data => {
            rows => scalar $player_score_rows->@*,
        }
    }, ScoreHandlerSuccess);
}


use constant BillingHandlerSuccess => SuccessResult({
    reports => json_type_arrayof(BillingReport)
});

# テナント管理者向けAPI
# GET /api/organizer/billing
# テナント内の課金レポートを取得する
sub billing_handler($self, $c) {
    my $v = $self->parse_viewer($c);
    unless ($v->{role} eq ROLE_ORGANIZER) {
        fail($c, HTTP_FORBIDDEN, "role organizer required");
    }

    my $tenant_db = connect_to_tenant_db($v->{tenant_id});
    defer { $tenant_db->disconnect }

    my $competitions = $tenant_db->select_all(
        "SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC",
        $v->{tenant_id},
    );

    my $tenant_billing_reports = [];
    for my $comp ($competitions->@*) {
        my $report = $self->billing_report_by_competition($c, $tenant_db, $v->{tenant_id}, $comp->{id});

        push $tenant_billing_reports->@*, $report;
    }

    return $c->render_json({
        status => true,
        data => {
            reports => $tenant_billing_reports,
        }
    }, BillingHandlerSuccess);
}

use constant PlayerScoreDetail => {
    competition_title => JSON_TYPE_STRING,
    score             => JSON_TYPE_INT,
};

use constant PlayerHandlerSuccess => SuccessResult({
    player => PlayerDetail,
    scores => json_type_arrayof(PlayerScoreDetail),
});

# 参加者向けAPI
# GET /api/player/player/:player_id
# 参加者の詳細情報を取得する
sub player_handler($self, $c) {
    my $v = $self->parse_viewer($c);
    unless ($v->{role} eq ROLE_PLAYER) {
        fail($c, HTTP_FORBIDDEN, "role player required");
    }

    my $tenant_db = connect_to_tenant_db($v->{tenant_id});
    defer { $tenant_db->disconnect }

    $self->authorize_player($c, $tenant_db, $v->{player_id});

    my $player_id = $c->args->{player_id};
    unless ($player_id) {
        fail($c, HTTP_BAD_REQUEST, "player_id is required");
    }

    my ($player, $err) = $self->retrieve_player($c, $tenant_db, $player_id);
    if ($err) {
        fail($c, HTTP_NOT_FOUND, "player not found");
    }

    my $competitions = $tenant_db->select_all(
        "SELECT * FROM competition WHERE tenant_id = ? ORDER BY created_at ASC",
        $v->{tenant_id},
    );

    # player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
    my $fl = flock_by_tenant_id($v->{tenant_id});
    defer { close $fl }

    my $player_scores = [];
    for my $comp ($competitions->@*) {
        my $player_score = $tenant_db->select_row(
            # 最後にCSVに登場したスコアを採用する = row_numが一番大きいもの
            "SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? AND player_id = ? ORDER BY row_num DESC LIMIT 1",
            $v->{tenant_id}, $comp->{id}, $player->{id},
        );
        unless ($player_score) {
            # 行がない = スコアが記録されてない
            next;
        }

        push $player_scores->@*, $player_score;
    }

    my $player_score_details = [];
    for my $player_score ($player_scores->@*) {
        my ($comp, $err) = $self->retrieve_competition($c, $tenant_db, $player_score->{competition_id});
        if ($err) {
            fail($c, HTTP_INTERNAL_SERVER_ERROR, sprintf("error retrieveCompetition: %s", $err));
        }
        push $player_score_details->@* => {
            competition_title => $comp->{title},
            score             => $player_score->{score},
        }
    }

    return $c->render_json({
        status => true,
        data => {
            player => {
                id => $player->{id},
                display_name => $player->{display_name},
                is_disqualified => $player->{is_disqualified},
            },
            scores => $player_score_details,
        }
    }, PlayerHandlerSuccess);
}


use constant CompetitionRank => {
    rank                => JSON_TYPE_INT,
    score               => JSON_TYPE_INT,
    player_id           => JSON_TYPE_STRING,
    player_display_name => JSON_TYPE_STRING,
    row_num             => undef, # # APIレスポンスのJSONには含まれない
};

use constant CompetitionRankingHandlerSuccess => SuccessResult({
    competition => CompetitionDetail,
    ranks       => json_type_arrayof(CompetitionRank),
});

# 参加者向けAPI
# GET /api/player/competition/:competition_id/ranking
# 大会ごとのランキングを取得する
sub competition_ranking_handler($self, $c) {
    my $v = $self->parse_viewer($c);
    unless ($v->{role} eq ROLE_PLAYER) {
        fail($c, HTTP_FORBIDDEN, "role player required");
    }

    my $tenant_db = connect_to_tenant_db($v->{tenant_id});
    defer { $tenant_db->disconnect }

    $self->authorize_player($c, $tenant_db, $v->{player_id});

    my $competition_id = $c->args->{competition_id};
    unless ($competition_id) {
        fail($c, HTTP_BAD_REQUEST, "competition_id is required");
    }

    # 大会の存在確認
    my ($competition, $err) = $self->retrieve_competition($c, $tenant_db, $competition_id);
    if ($err) {
        fail($c, HTTP_NOT_FOUND, "competition not found")
    }

    my $now = time;

    my $tenant = $self->admin_db->select_row(
        "SELECT * FROM tenant WHERE id = ?",
        $v->{tenant_id},
    );

    $self->admin_db->query(
        "INSERT INTO visit_history (player_id, tenant_id, competition_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
        $v->{player_id}, $tenant->{id}, $competition_id, $now, $now,
    );

    my $rank_after = $c->request->query_parameters->{rank_after} || 0;

    # player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
    my $fl = flock_by_tenant_id($v->{tenant_id});
    defer { close $fl }

    my $player_scores = $tenant_db->select_all(
        "SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? ORDER BY row_num DESC",
        $tenant->{id}, $competition_id,
    );

    my $ranks = [];
    my $scored_player_set = {};

    for my $player_score ($player_scores->@*) {
        # player_scoreが同一player_id内ではrow_numの降順でソートされているので
        # 現れたのが2回目以降のplayer_idはより大きいrow_numでスコアが出ているとみなせる
        if (exists $scored_player_set->{$player_score->{player_id}}) {
            next;
        }

        $scored_player_set->{$player_score->{player_id}} = !!1;
        my ($player, $err) = $self->retrieve_player($c, $tenant_db, $player_score->{player_id});
        if ($err) {
            fail($c, HTTP_INTERNAL_SERVER_ERROR, sprintf("error retrieve_player: %s", $err));
        }

        push $ranks->@* => {
            score               => $player_score->{score},
            player_id           => $player->{id},
            player_display_name => $player->{display_name},
            row_num             => $player_score->{row_num},
        };
    }

    my @sorted_ranks = sort {
        if ($a->{score} == $b->{score}) {
            return $a->{row_num} <=> $b->{row_num}
        }
        return $b->{score} <=> $a->{score}
    } $ranks->@*;

    my $page_ranks = [];
    for (my $i = 0; $i < @sorted_ranks; $i++) {
        my $rank = $sorted_ranks[$i];

        if ($i < $rank_after) {
            next;
        }

        push $page_ranks->@* => {
            rank                => $i + 1,
            score               => $rank->{score},
            player_id           => $rank->{player_id},
            player_display_name => $rank->{player_display_name},
        };

        if ($page_ranks->@* >= 100) {
            last;
        }
    }

    return $c->render_json({
        status => true,
        data => {
            competition => {
                id          => $competition->{id},
                title       => $competition->{title},
                is_finished => !!$competition->{finished_at},
            },
            ranks => $page_ranks,
        }
    }, CompetitionRankingHandlerSuccess);
}

use constant CompetitionsHandlerSuccess => SuccessResult({
    competitions => json_type_arrayof(CompetitionDetail),
});

# 参加者向けAPI
# GET /api/player/competitions
# 大会の一覧を取得する
sub player_competitions_handler($self, $c) {
    my $v = $self->parse_viewer($c);
    unless ($v->{role} eq ROLE_PLAYER) {
        fail($c, HTTP_FORBIDDEN, "role player required");
    }

    my $tenant_db = connect_to_tenant_db($v->{tenant_id});
    defer { $tenant_db->disconnect }

    $self->authorize_player($c, $tenant_db, $v->{player_id});

    return competitions_handler($c, $v, $tenant_db);
}

# テナント管理者向けAPI
# GET /api/organizer/competitions
# 大会の一覧を取得する
sub organizer_competitions_handler($self, $c) {
    my $v = $self->parse_viewer($c);
    unless ($v->{role} eq ROLE_ORGANIZER) {
        fail($c, HTTP_FORBIDDEN, "role organizer required");
    }

    my $tenant_db = connect_to_tenant_db($v->{tenant_id});
    defer { $tenant_db->disconnect }

    return competitions_handler($c, $v, $tenant_db);
}

sub competitions_handler($c, $viewer, $tenant_db) {
    my $competitions = $tenant_db->select_all(
        "SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC",
        $viewer->{tenant_id},
    );

    my $competition_details = [];
    for my $comp ($competitions->@*) {
        push $competition_details->@* => {
            id          => $comp->{id},
            title       => $comp->{title},
            is_finished => !!$comp->{finished_at},
        };
    }

    return $c->render_json({
        status => true,
        data => {
            competitions => $competition_details,
        }
    }, CompetitionsHandlerSuccess);
}

use constant TenantDetail => {
    name         => JSON_TYPE_STRING,
    display_name => JSON_TYPE_STRING,
};

use constant MeHandlerSuccess => SuccessResult({
    tenant    => TenantDetail,
    me        => json_type_null_or_anyof(PlayerDetail),
    role      => JSON_TYPE_STRING,
    logged_in => JSON_TYPE_BOOL,
});

# 共通API
# GET /api/me
# JWTで認証した結果、テナントやユーザ情報を返す
sub me_handler($self, $c) {
    my ($tenant, $err) = $self->retrieve_tenant_row_from_header($c);
    if ($err) {
        fail($c, HTTP_INTERNAL_SERVER_ERROR, sprintf("error retrieve_tenant_row_from_header: %s", $err));
    }
    my $tenant_detail = {
        name         => $tenant->{name},
        display_name => $tenant->{display_name},
    };

    my $v;
    try {
        $v = $self->parse_viewer($c);
    }
    catch ($e) {
        if ($e isa Kossy::Exception && $e->{code} == HTTP_UNAUTHORIZED) {
            return $c->render_json({
                status => true,
                data => {
                    tenant    => $tenant_detail,
                    me        => undef,
                    role      => ROLE_NONE,
                    logged_in => false,
                }
            }, MeHandlerSuccess);
        }

        fail($c, HTTP_INTERNAL_SERVER_ERROR, sprintf('error parse viewer: %s', $e));
    }

    if ($v->{role} eq ROLE_ADMIN || $v->{role} eq ROLE_ORGANIZER) {
        return $c->render_json({
            status => true,
            data => {
                tenant    => $tenant_detail,
                me        => undef,
                role      => $v->{role},
                logged_in => true,
            }
        }, MeHandlerSuccess);
    }

    my $tenant_db = connect_to_tenant_db($v->{tenant_id});
    defer { $tenant_db->disconnect }

    (my $player, $err) = $self->retrieve_player($c, $tenant_db, $v->{player_id});
    if ($err) {
        return $c->render_json({
            status => true,
            data => {
                tenant    => $tenant_detail,
                me        => undef,
                role      => ROLE_NONE,
                logged_in => false,
            }
        }, MeHandlerSuccess);
    }

    return $c->render_json({
        status => true,
        data => {
            tenant => $tenant_detail,
            me => {
                id              => $player->{id},
                display_name    => $player->{display_name},
                is_disqualified => $player->{is_disqualified},
            },
            role      => $v->{role},
            logged_in => true,
        }
    }, MeHandlerSuccess);
}

use constant InitializeHandlerSuccess => SuccessResult({
    lang => JSON_TYPE_STRING,
});

# ベンチマーカー向けAPI
# POST /initialize
# ベンチマーカーが起動したときに最初に呼ぶ
# データベースの初期化などが実行されるため、スキーマを変更した場合などは適宜改変すること
sub initialize_handler($self, $c) {
    my $e = system(INITIALIZE_SCRIPT);
    if ($e) {
        fail($c, HTTP_INTERNAL_SERVER_ERROR, "error exec.Command: %s", $e);
    }

    return $c->render_json({
        status => true,
        data => {
            lang => "perl",
        }
    }, InitializeHandlerSuccess);
}

1;
