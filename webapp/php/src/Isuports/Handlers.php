<?php

declare(strict_types=1);

namespace App\Isuports;

use Doctrine\DBAL\Configuration;
use Doctrine\DBAL\Connection;
use Doctrine\DBAL\DriverManager;
use Doctrine\DBAL\Exception as DBException;
use Firebase\JWT\JWT;
use Firebase\JWT\Key;
use JsonSerializable;
use PDO;
use Psr\Http\Message\ResponseInterface as Response;
use Psr\Http\Message\ServerRequestInterface as Request;
use Psr\Log\LoggerInterface as Logger;
use RuntimeException;
use Slim\Exception\HttpBadRequestException;
use Slim\Exception\HttpForbiddenException;
use Slim\Exception\HttpNotFoundException;
use Slim\Exception\HttpUnauthorizedException;
use UnexpectedValueException;

class Handlers
{
    private const TENANT_DB_SCHEMA_FILE_PATH = __DIR__ . '/../../../sql/tenant/10_schema.sql';
    private const INITIALIZE_SCRIPT = __DIR__ . '/../../../sql/init.sh';
    private const COOKIE_NAME = 'isuports_session';

    private const ROLE_ADMIN = 'admin';
    private const ROLE_ORGANIZER = 'organizer';
    private const ROLE_PLAYER = 'player';
    private const ROLE_NONE = 'none';

    // 正しいテナント名の正規表現
    private const TENANT_NAME_REGEXP = '/^[a-z][a-z0-9-]{0,61}[a-z0-9]$/';

    public function __construct(
        private Connection $adminDB,
        private Configuration $sqliteConfiguration, // sqliteのクエリログを出力する設定
        private Logger $logger, // 初期実装では使っていませんがデバッグ用にお使いください
    ) {
    }

    /**
     * テナントDBのパスを返す
     */
    private function tenantDBPath(int $id): string
    {
        $tenantDBDir = getenv('ISUCON_TENANT_DB_DIR') ?: __DIR__ . '/../../../tenant_db';

        return $tenantDBDir . DIRECTORY_SEPARATOR . sprintf('%d.db', $id);
    }

    /**
     * テナントDBに接続する
     */
    private function connectToTenantDB(int $id): Connection
    {
        return DriverManager::getConnection(
            params: [
                'path' => $this->tenantDBPath($id),
                'driver' => 'pdo_sqlite',
            ],
            config: $this->sqliteConfiguration,
        );
    }

    /**
     * テナントDBを新規に作成する
     */
    private function createTenantDB(int $id): void
    {
        $p = $this->tenantDBPath($id);

        $cmd =  ['sh', '-c', sprintf('sqlite3 %s < %s', $p, self::TENANT_DB_SCHEMA_FILE_PATH)];
        if ($this->execCommand($cmd, $out) !== 0) {
            throw new RuntimeException(sprintf('failed to exec sqlite3 %s < %s, out=%s', $p, self::TENANT_DB_SCHEMA_FILE_PATH, $out));
        }
    }

    /**
     * システム全体で一意なIDを生成する
     */
    private function dispenseID(): string
    {
        $id = 0;
        /** @var ?\Exception $lastErr */
        $lastErr = null;
        for ($i = 0; $i < 100; $i++) {
            try {
                $this->adminDB->prepare('REPLACE INTO id_generator (stub) VALUES (?);')
                    ->executeStatement(['a']);
            } catch (DBException $e) {
                if ($e->getCode() === 1213) { // deadlock
                    $lastErr = $e;
                    continue;
                }
                throw $e;
            }

            $id = $this->adminDB->lastInsertId();
            break;
        }

        if ($id !== 0) {
            return sprintf('%x', $id);
        }

        throw $lastErr;
    }

    /**
     * リクエストヘッダをパースしてViewerを返す
     */
    private function parseViewer(Request $request): Viewer
    {
        $tokenStr = $request->getCookieParams()[self::COOKIE_NAME] ?? '';
        if ($tokenStr === '') {
            throw new HttpUnauthorizedException($request, sprintf('cookie %s is not found', self::COOKIE_NAME));
        }

        $keyFilename = getenv('ISUCON_JWT_KEY_FILE') ?: __DIR__ . '/../../../public.pem';
        $keysrc = file_get_contents($keyFilename);
        if ($keysrc === false) {
            throw new RuntimeException(sprintf('error file_get_contents: keyFilename=%s', $keyFilename));
        }

        $key = new Key($keysrc, 'RS256');

        try {
            $token = JWT::decode($tokenStr, $key);
        } catch (UnexpectedValueException $e) {
            throw new HttpUnauthorizedException($request, $e->getMessage(), $e);
        }

        if ($token->sub == '') {
            throw new HttpUnauthorizedException($request, sprintf('invalid token: subject is not found in token: %s', $tokenStr));
        }

        if (!property_exists($token, 'role')) {
            throw new HttpUnauthorizedException($request, sprintf('invalid token: role is not found in token: %s', $tokenStr));
        }

        /** @var string $role */
        $role = match ($token->role) {
            self::ROLE_ADMIN, self::ROLE_ORGANIZER, self::ROLE_PLAYER => $token->role,
            default => throw new HttpUnauthorizedException($request, sprintf('invalid token: %s is invalid role: %s', $token->role, $tokenStr)),
        };

        /** @var list<string> $aud */
        $aud = $token->aud;
        if (count($aud) !== 1) {
            throw new HttpUnauthorizedException($request, sprintf('invalid token: aud field is few or too much: %s', $tokenStr));
        }

        $tenant = $this->retrieveTenantRowFromHeader($request);

        if (is_null($tenant)) {
            throw new HttpUnauthorizedException($request, 'tenant not found');
        }

        if ($tenant->name === 'admin' && $role !== self::ROLE_ADMIN) {
            throw new HttpUnauthorizedException($request, 'tenant not found');
        }

        if ($tenant->name !== $aud[0]) {
            throw new HttpUnauthorizedException(
                $request,
                sprintf('invalid token: tenant name is not match with %s: %s', $request->getHeader('Host')[0], $tokenStr),
            );
        }

        return new Viewer(
            role: $role,
            playerID: $token->sub,
            tenantName: $tenant->name,
            tenantID: $tenant->id,
        );
    }

    private function retrieveTenantRowFromHeader(Request $request): ?TenantRow
    {
        // JWTに入っているテナント名とHostヘッダのテナント名が一致しているか確認
        $baseHost = getenv('ISUCON_BASE_HOSTNAME') ?: '.t.isucon.dev';
        $tenantName = preg_replace('/' . preg_quote($baseHost) . '$/', '', $request->getHeader('Host')[0]);

        // SaaS管理者用ドメイン
        if ($tenantName === 'admin') {
            return new TenantRow(
                name:'admin',
                displayName: 'admin'
            );
        }

        // テナントの存在確認
        $row = $this->adminDB->prepare('SELECT * FROM tenant WHERE name = ?')
            ->executeQuery([$tenantName])
            ->fetchAssociative();

        if ($row === false) {
            return null;
        }

        return new TenantRow(
            id: $row['id'],
            name: $row['name'],
            displayName: $row['display_name'],
            createdAt: $row['created_at'],
            updatedAt: $row['updated_at'],
        );
    }

    /**
     * 参加者を取得する
     */
    private function retrievePlayer(Connection $tenantDB, string $id): ?PlayerRow
    {
        $row = $tenantDB->prepare('SELECT * FROM player WHERE id = ?')
            ->executeQuery([$id])
            ->fetchAssociative();

        if ($row === false) {
            return null;
        }

        return new PlayerRow(
            tenantID: $row['tenant_id'],
            id: $row['id'],
            displayName: $row['display_name'],
            isDisqualified: (bool)$row['is_disqualified'],
            createdAt: $row['created_at'],
            updatedAt: $row['updated_at'],
        );
    }

    /**
     * 参加者を認可する
     * 参加者向けAPIで呼ばれる
     */
    private function authorizePlayer(Request $request, Connection $tenantDB, string $id): void
    {
        $player = $this->retrievePlayer($tenantDB, $id);

        if (is_null($player)) {
            throw new HttpUnauthorizedException($request, 'player not found');
        }

        if ($player->isDisqualified) {
            throw new HttpForbiddenException($request, 'player is disqualified');
        }
    }

    /**
     * 大会を取得する
     */
    private function retrieveCompetition(Connection $tenantDB, string $id): ?CompetitionRow
    {
        $row = $tenantDB->prepare('SELECT * FROM competition WHERE id = ?')
            ->executeQuery([$id])
            ->fetchAssociative();

        if ($row === false) {
            return null;
        }

        return new CompetitionRow(
            tenantID: $row['tenant_id'],
            id: $row['id'],
            title: $row['title'],
            finishedAt: is_null($row['finished_at']) ? null : $row['finished_at'],
            createdAt: $row['created_at'],
            updatedAt: $row['updated_at'],
        );
    }

    /**
     * 排他ロックのためのファイル名を生成する
     */
    private function lockFilePath(int $id): string
    {
        $tenantDBDir = getenv('ISUCON_TENANT_DB_DIR') ?: __DIR__ . '/../../../tenant_db';

        return $tenantDBDir . DIRECTORY_SEPARATOR . sprintf('%d.lock', $id);
    }

    /**
     * 排他ロックする
     *
     * @return resource
     */
    private function flockByTenantID(int $tenantID): mixed
    {
        $p = $this->lockFilePath($tenantID);

        /** @var resource $fl */
        $fl = fopen($p, 'w+');
        if (flock($fl, LOCK_EX) === false) {
            throw new RuntimeException(sprintf('error flock.Lock: path=%s', $p));
        }

        return $fl;
    }

    /**
     * SasS管理者用API
     * テナントを追加する
     * POST /api/admin/tenants/add
     */
    public function tenantsAddHandler(Request $request, Response $response): Response
    {
        $v = $this->parseViewer($request);
        if ($v->tenantName !== 'admin') {
            throw new HttpNotFoundException($request, sprintf('%s has not this API', $v->tenantName));
        }

        if ($v->role !== self::ROLE_ADMIN) {
            throw new HttpForbiddenException($request, 'admin role required');
        }

        $formValue = $request->getParsedBody();
        $displayName = $formValue['display_name'] ?? '';
        $name = $formValue['name'] ?? '';
        if (!$this->validateTenantName($name)) {
            throw new HttpBadRequestException($request, sprintf('invalid tenant name: %s', $name));
        }

        $now = time();
        try {
            $this->adminDB->prepare('INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)')
                ->executeStatement([$name, $displayName, $now, $now]);
        } catch (DBException $e) {
            if ($e->getCode() === 1062) { // duplicate entry
                throw new HttpBadRequestException($request, 'duplicate tenant', $e);
            }

            throw $e;
        }

        /** @var string $id */
        $id = $this->adminDB->lastInsertId();

        // NOTE: 先にadminDBに書き込まれることでこのAPIの処理中に
        //       /api/admin/tenants/billingにアクセスされるとエラーになりそう
        //       ロックなどで対処したほうが良さそう
        $this->createTenantDB((int)$id);

        $res = new TenantsAddHandlerResult(
            tenant: new TenantWithBilling(
                id: $id,
                name: $name,
                displayName: $displayName,
                billingYen: 0,
            ),
        );

        return $this->jsonResponse($response, new SuccessResult(success: true, data: $res));
    }

    /**
     * テナント名が規則に沿っているかチェックする
     */
    private function validateTenantName(string $name): bool
    {
        return preg_match(self::TENANT_NAME_REGEXP, $name) === 1;
    }

    /**
     * 大会ごとの課金レポートを計算する
     */
    private function billingReportByCompetition(Connection $tenantDB, int $tenantID, string $competitionID): BillingReport
    {
        $comp = $this->retrieveCompetition($tenantDB, $competitionID);
        if (is_null($comp)) {
            throw new RuntimeException('error retrieveCompetition');
        }

        // ランキングにアクセスした参加者のIDを取得する
        $vhs = $this->adminDB->prepare('SELECT player_id, MIN(created_at) AS min_created_at FROM visit_history WHERE tenant_id = ? AND competition_id = ? GROUP BY player_id')
            ->executeQuery([$tenantID, $comp->id])
            ->fetchAllAssociative();

        /** @var array<string, string> $billingMap */
        $billingMap = [];
        foreach ($vhs as $vh) {
            // competition.finished_atよりもあとの場合は、終了後に訪問したとみなして大会開催内アクセス済みとみなさない
            if (!is_null($comp->finishedAt) && $comp->finishedAt < $vh['min_created_at']) {
                continue;
            }
            $billingMap[$vh['player_id']] = 'visitor';
        }

        // player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
        $fl = $this->flockByTenantID($tenantID);

        // スコアを登録した参加者のIDを取得する
        $scoredPlayerIDs = $tenantDB->prepare('SELECT DISTINCT(player_id) FROM player_score WHERE tenant_id = ? AND competition_id = ?')
            ->executeQuery([$tenantID, $comp->id])
            ->fetchFirstColumn();
        foreach ($scoredPlayerIDs as $pid) {
            // スコアが登録されている参加者
            $billingMap[$pid] = 'player';
        }

        // 大会が終了している場合のみ請求金額が確定するので計算する
        $playerCount = 0;
        $visitorCount = 0;
        if (!is_null($comp->finishedAt)) {
            $counts = array_count_values($billingMap);
            $playerCount = $counts['player'] ?? 0;
            $visitorCount =  $counts['visitor'] ?? 0;
        }

        fclose($fl);

        return new BillingReport(
            competitionID: $comp->id,
            competitionTitle: $comp->title,
            playerCount: $playerCount,
            visitorCount: $visitorCount,
            billingPlayerYen: 100 * $playerCount, // スコアを登録した参加者は100円
            billingVisitorYen: 10 * $visitorCount, // ランキングを閲覧だけした(スコアを登録していない)参加者は10円
            billingYen: 100 * $playerCount + 10 * $visitorCount,
        );
    }

    /**
     * SaaS管理者用API
     * テナントごとの課金レポートを最大20件、テナントのid降順で取得する
     * POST /api/admin/tenants/billing
     * URL引数beforeを指定した場合、指定した値よりもidが小さいテナントの課金レポートを取得する
     */
    public function tenantsBillingHandler(Request $request, Response $response): Response
    {
        $host = $request->getHeader('Host')[0] ?? '';
        if ($host !== (getenv('ISUCON_ADMIN_HOSTNAME') ?: 'admin.t.isucon.dev')) {
            throw new HttpNotFoundException($request, sprintf('invalid hostname %s', $host));
        }

        $v = $this->parseViewer($request);
        if ($v->role !== self::ROLE_ADMIN) {
            throw new HttpForbiddenException($request, 'admin role required');
        }

        $beforeID = 0;
        $before = $request->getQueryParams()['before'] ?? '';
        if ($before !== '') {
            $beforeID = filter_var($before, FILTER_VALIDATE_INT);
            if (!is_int($beforeID)) {
                throw new HttpBadRequestException($request, sprintf('failed to parse query parameter \'before\': %s', $before));
            }
        }

        // テナントごとに
        //   大会ごとに
        //     scoreが登録されているplayer * 100
        //     scoreが登録されていないplayerでアクセスした人 * 10
        //   を合計したものを
        // テナントの課金とする
        $ts = $this->adminDB->executeQuery('SELECT * FROM tenant ORDER BY id DESC')
            ->fetchAllAssociative();

        /** @var list<TenantWithBilling> $tenantBillings */
        $tenantBillings = [];
        foreach ($ts as $t) {
            if ($beforeID !== 0 && $beforeID <= $t['id']) {
                continue;
            }

            $tb = new TenantWithBilling(
                id: (string)$t['id'],
                name: $t['name'],
                displayName: $t['display_name'],
            );

            $tenantDB = $this->connectToTenantDB($t['id']);
            $cs = $tenantDB->prepare('SELECT * FROM competition WHERE tenant_id=?')
                ->executeQuery([$t['id']])
                ->fetchAllAssociative();

            foreach ($cs as $comp) {
                $report = $this->billingReportByCompetition($tenantDB, $t['id'], $comp['id']);
                $tb->billingYen += $report->billingYen;
            }

            $tenantBillings[] = $tb;

            $tenantDB->close();

            if (count($tenantBillings) >= 10) {
                break;
            }
        }

        return $this->jsonResponse($response, new SuccessResult(
            success: true,
            data: new TenantsBillingHandlerResult(
                tenants: $tenantBillings,
            ),
        ));
    }

    /**
     * テナント管理者向けAPI
     * GET /api/organizer/players
     * 参加者一覧を返す
     */
    public function playersListHandler(Request $request, Response $response): Response
    {
        $v = $this->parseViewer($request);

        if ($v->role !== self::ROLE_ORGANIZER) {
            throw new HttpForbiddenException($request, 'role organizer required');
        }

        $tenantDB = $this->connectToTenantDB($v->tenantID);

        /** @var list<PlayerDetail> $pds */
        $pds = [];
        $result = $tenantDB->prepare('SELECT * FROM player WHERE tenant_id=? ORDER BY created_at DESC')
            ->executeQuery([$v->tenantID]);
        while ($row = $result->fetchAssociative()) {
            $pds[] = new PlayerDetail(
                id: $row['id'],
                displayName: $row['display_name'],
                isDisqualified: (bool)$row['is_disqualified'],
            );
        }

        $res = new PlayersListHandlerResult(
            players: $pds,
        );

        $tenantDB->close();

        return $this->jsonResponse($response, new SuccessResult(success: true, data: $res));
    }

    /**
     * テナント管理者向けAPI
     * POST /api/organizer/players/add
     * テナントに参加者を追加する
     */
    public function playersAddHandler(Request $request, Response $response): Response
    {
        $v = $this->parseViewer($request);

        if ($v->role !== self::ROLE_ORGANIZER) {
            throw new HttpForbiddenException($request, 'role organizer required');
        }

        $tenantDB = $this->connectToTenantDB($v->tenantID);

        $params = $request->getParsedBody();
        if (!is_array($params)) {
            throw new RuntimeException('error getParsedBody');
        }
        /** @var list<string> $displayNames */
        $displayNames = $params['display_name'] ?? [];

        /** @var list<PlayerDetail> $pds */
        $pds = [];
        foreach ($displayNames as $displayName) {
            $id = $this->dispenseID();

            $now = time();
            $tenantDB->prepare('INSERT INTO player (id, tenant_id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)')
                ->executeStatement([$id, $v->tenantID, $displayName, false, $now, $now]);

            $p = $this->retrievePlayer($tenantDB, $id);

            $pds[] = new PlayerDetail(
                id: $p->id,
                displayName: $p->displayName,
                isDisqualified: $p->isDisqualified,
            );
        }

        $res = new PlayersAddHandlerResult(players: $pds);

        $tenantDB->close();

        return $this->jsonResponse($response, new SuccessResult(success: true, data: $res));
    }

    /**
     * テナント管理者向けAPI
     * POST /api/organizer/player/:player_id/disqualified
     * 参加者を失格にする
     */
    public function playerDisqualifiedHandler(Request $request, Response $response, array $params): Response
    {
        $v = $this->parseViewer($request);
        if ($v->role !== self::ROLE_ORGANIZER) {
            throw new HttpForbiddenException($request, 'role organizer required');
        }

        $tenantDB = $this->connectToTenantDB($v->tenantID);

        $playerID = $params['player_id'];

        $now = time();
        $tenantDB->prepare('UPDATE player SET is_disqualified = ?, updated_at = ? WHERE id = ?')
            ->executeStatement([true, $now, $playerID]);

        $p = $this->retrievePlayer($tenantDB, $playerID);
        if (is_null($p)) {
            // 存在しないプレイヤー
            throw new HttpNotFoundException($request, 'player not found');
        }

        $res = new PlayerDisqualifiedHandlerResult(
            player: new PlayerDetail(
                id: $p->id,
                displayName: $p->displayName,
                isDisqualified: $p->isDisqualified,
            ),
        );

        $tenantDB->close();

        return $this->jsonResponse($response, new SuccessResult(success: true, data: $res));
    }

    /**
     * テナント管理者向けAPI
     * POST /api/organizer/competitions/add
     * 大会を追加する
     */
    public function competitionsAddHandler(Request $request, Response $response): Response
    {
        $v = $this->parseViewer($request);

        $tenantDB = $this->connectToTenantDB($v->tenantID);

        $title = $request->getParsedBody()['title'] ?? '';

        $now = time();
        $id = $this->dispenseID();

        $tenantDB->prepare('INSERT INTO competition (id, tenant_id, title, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)')
            ->executeStatement([$id, $v->tenantID, $title, null, $now, $now]);

        $res = new CompetitionsAddHandlerResult(
            competition: new CompetitionDetail(
                id: $id,
                title: $title,
                isFinished: false,
            ),
        );

        $tenantDB->close();

        return $this->jsonResponse($response, new SuccessResult(success: true, data: $res));
    }

    /**
     * テナント管理者向けAPI
     * POST /api/organizer/competition/:competition_id/finish
     * 大会を終了する
     */
    public function competitionFinishHandler(Request $request, Response $response, array $params): Response
    {
        $v = $this->parseViewer($request);
        if ($v->role !== self::ROLE_ORGANIZER) {
            throw new HttpForbiddenException($request, 'role organizer required');
        }

        $tenantDB = $this->connectToTenantDB($v->tenantID);
        $id = $params['competition_id'] ?? '';
        if ($id === '') {
            throw new HttpBadRequestException($request, 'competition_id required');
        }

        // 存在しない大会
        if (is_null($this->retrieveCompetition($tenantDB, $id))) {
            throw new HttpNotFoundException($request, 'competition not found');
        }

        $now = time();
        $tenantDB->prepare('UPDATE competition SET finished_at = ?, updated_at = ? WHERE id = ?')
            ->executeStatement([$now, $now, $id]);

        return $this->jsonResponse($response, new SuccessResult(success: true));
    }

    /**
     * テナント管理者向けAPI
     * POST /api/organizer/competition/:competition_id/score
     * 大会のスコアをCSVでアップロードする
     */
    public function competitionScoreHandler(Request $request, Response $response, array $params): Response
    {
        $v = $this->parseViewer($request);

        if ($v->role !== self::ROLE_ORGANIZER) {
            throw new HttpForbiddenException($request, 'role organizer required');
        }

        $tenantDB = $this->connectToTenantDB($v->tenantID);

        $competitionID = $params['competition_id'] ?? '';
        if ($competitionID === '') {
            throw new HttpBadRequestException($request, 'competition_id required');
        }

        $comp = $this->retrieveCompetition($tenantDB, $competitionID);

        if (is_null($comp)) {
            // 存在しない大会
            throw new HttpNotFoundException($request, 'competition not found');
        }

        if (!is_null($comp->finishedAt)) {
            $res = new FailureResult(
                success: false,
                message: 'competition is finished',
            );
            return $this->jsonResponse($response, $res, 400);
        }

        /** @var \Psr\Http\Message\UploadedFileInterface|null $uploadedFile */
        $uploadedFile = $request->getUploadedFiles()['scores'] ?? null;
        if (is_null($uploadedFile) || $uploadedFile->getError() !== UPLOAD_ERR_OK) {
            throw new RuntimeException('error getUploadedFiles');
        }

        $tmpFilePath = tempnam(sys_get_temp_dir(), '');
        $uploadedFile->moveTo($tmpFilePath);
        $fh = fopen($tmpFilePath, 'r');
        if ($fh === false) {
            throw new RuntimeException(sprintf('error fopen: %s', $tmpFilePath));
        }

        try {
            $headers = fgetcsv($fh);
            if ($headers === false) {
                throw new RuntimeException('error fgetcsv at header');
            }
            if ($headers != ['player_id', 'score']) {
                throw new HttpBadRequestException($request, 'invalid CSV headers');
            }

            // / DELETEしたタイミングで参照が来ると空っぽのランキングになるのでロックする
            $fl = $this->flockByTenantID($v->tenantID);

            $rowNum = 0;
            /** @var list<array<string, mixed>> $playerScoreRows */
            $playerScoreRows = [];
            while (($row = fgetcsv($fh)) !== false) {
                $rowNum++;

                if (count($row) !== 2) {
                    throw new RuntimeException(sprintf('row must have two columns: %s', var_export($row, true)));
                }

                [$playerID, $scoreStr] = $row;
                // 存在しない参加者が含まれている
                if (is_null($this->retrievePlayer($tenantDB, $playerID))) {
                    throw new HttpBadRequestException($request, sprintf('player not found: %s', $playerID));
                }

                $id = $this->dispenseID();
                $score = filter_var($scoreStr, FILTER_VALIDATE_INT);
                if (!is_int($score)) {
                    throw new HttpBadRequestException($request, sprintf('error filter_var: scoreStr=%s', $scoreStr));
                }
                $now = time();

                $playerScoreRows[] = [
                    'id' => $id,
                    'tenant_id' => $v->tenantID,
                    'player_id' => $playerID,
                    'competition_id' => $competitionID,
                    'score' => $score,
                    'row_num' => $rowNum,
                    'created_at' => $now,
                    'updated_at' => $now,
                ];
            }
        } finally {
            fclose($fh);
            unlink($tmpFilePath);
        }

        $tenantDB->prepare('DELETE FROM player_score WHERE tenant_id = ? AND competition_id = ?')
            ->executeStatement([$v->tenantID, $competitionID]);

        foreach ($playerScoreRows as $ps) {
            $tenantDB->prepare('INSERT INTO player_score (id, tenant_id, player_id, competition_id, score, row_num, created_at, updated_at) VALUES (:id, :tenant_id, :player_id, :competition_id, :score, :row_num, :created_at, :updated_at)')
                ->executeStatement($ps);
        }

        $tenantDB->close();
        fclose($fl);

        return $this->jsonResponse($response, new SuccessResult(
            success: true,
            data: new ScoreHandlerResult(rows: count($playerScoreRows)),
        ));
    }

    /**
     * テナント管理者向けAPI
     * GET /api/organizer/billing
     * テナント内の課金レポートを取得する
     */
    public function billingHandler(Request $request, Response $response): Response
    {
        $v = $this->parseViewer($request);
        if ($v->role !== self::ROLE_ORGANIZER) {
            throw new HttpForbiddenException($request, 'role organizer required');
        }

        $tenantDB = $this->connectToTenantDB($v->tenantID);

        $cs = $tenantDB->prepare('SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC')
            ->executeQuery([$v->tenantID])
            ->fetchAllAssociative();
        if (count($cs) === 0) {
            throw new RuntimeException('error Select competition');
        }

        /** @var list<BillingReport> $tbrs */
        $tbrs = [];
        foreach ($cs as $comp) {
            $tbrs[] = $this->billingReportByCompetition($tenantDB, $v->tenantID, $comp['id']);
        }

        $res = new SuccessResult(
            success: true,
            data: new BillingHandlerResult(
                reports: $tbrs,
            ),
        );

        $tenantDB->close();

        return $this->jsonResponse($response, $res);
    }

    /**
     * 参加者向けAPI
     * GET /api/player/player/:player_id
     * 参加者の詳細情報を取得する
     */
    public function playerHandler(Request $request, Response $response, array $params): Response
    {
        $v = $this->parseViewer($request);
        if ($v->role !== self::ROLE_PLAYER) {
            throw new HttpForbiddenException($request, 'role player required');
        }

        $tenantDB = $this->connectToTenantDB($v->tenantID);

        $this->authorizePlayer($request, $tenantDB, $v->playerID);

        $playerID = $params['player_id'] ?? '';
        if ($playerID === '') {
            throw new HttpBadRequestException($request, 'player_id is required');
        }

        $p = $this->retrievePlayer($tenantDB, $playerID);
        if (is_null($p)) {
            throw new HttpNotFoundException($request, 'player not found');
        }

        $cs = $tenantDB->prepare('SELECT * FROM competition WHERE tenant_id = ? ORDER BY created_at ASC')
            ->executeQuery([$v->tenantID])
            ->fetchAllAssociative();

        // player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
        $fl = $this->flockByTenantID($v->tenantID);

        $pss = [];
        foreach ($cs as $c) {
            $ps = $tenantDB->prepare('SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? AND player_id = ? ORDER BY row_num DESC LIMIT 1')
                ->executeQuery([$v->tenantID, $c['id'], $p->id])
                ->fetchAssociative();
            // 行がない = スコアが記録されてない
            if ($ps === false) {
                continue;
            }

            $pss[] = $ps;
        }

        /** @var list<PlayerScoreDetail> $psds */
        $psds = [];
        foreach ($pss as $ps) {
            $comp = $this->retrieveCompetition($tenantDB, $ps['competition_id']);
            if (is_null($comp)) {
                throw new RuntimeException('error retrieveCompetition');
            }

            $psds[] = new PlayerScoreDetail(
                competitionTitle: $comp->title,
                score: $ps['score'],
            );
        }

        $res = new SuccessResult(
            success: true,
            data: new PlayerHandlerResult(
                player: new PlayerDetail(
                    id: $p->id,
                    displayName: $p->displayName,
                    isDisqualified: $p->isDisqualified,
                ),
                scores: $psds,
            ),
        );

        $tenantDB->close();
        fclose($fl);

        return $this->jsonResponse($response, $res);
    }

    /**
     * 参加者向けAPI
     * GET /api/player/competition/:competition_id/ranking
     * 大会ごとのランキングを取得する
     */
    public function competitionRankingHandler(Request $request, Response $response, array $params): Response
    {
        $v = $this->parseViewer($request);

        if ($v->role !== self::ROLE_PLAYER) {
            throw new HttpForbiddenException($request, 'role player required');
        }

        $tenantDB = $this->connectToTenantDB($v->tenantID);

        $this->authorizePlayer($request, $tenantDB, $v->playerID);

        $competitionID = $params['competition_id'] ?? '';
        if ($competitionID === '') {
            throw new HttpBadRequestException($request, 'competition_id is required');
        }

        // 大会の存在確認
        $competition = $this->retrieveCompetition($tenantDB, $competitionID);
        if (is_null($competition)) {
            throw new HttpNotFoundException($request, 'competition not found');
        }

        $now = time();
        $tenant = $this->adminDB->prepare('SELECT * FROM tenant WHERE id = ?')
            ->executeQuery([$v->tenantID])
            ->fetchAssociative();
        if ($tenant === false) {
            throw new RuntimeException(sprintf('error Select tenant: id=%d', $v->tenantID));
        }

        $this->adminDB->prepare('INSERT INTO visit_history (player_id, tenant_id, competition_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)')
            ->executeStatement([$v->playerID, $tenant['id'], $competitionID, $now, $now]);

        $rankAfter = 0;
        $rankAfterStr = $request->getQueryParams()['rank_after'] ?? '';
        if ($rankAfterStr !== '') {
            $rankAfter = filter_var($rankAfterStr, FILTER_VALIDATE_INT);
            if (!is_int($rankAfter)) {
                throw new RuntimeException(sprintf('error filter_var: rankAfterStr=%s', $rankAfterStr));
            }
        }

        // player_scoreを読んでいるときに更新が走ると不整合が起こるのでロックを取得する
        $fl = $this->flockByTenantID($v->tenantID);

        $pss = $tenantDB->prepare('SELECT * FROM player_score WHERE tenant_id = ? AND competition_id = ? ORDER BY row_num DESC')
            ->executeQuery([$tenant['id'], $competitionID])
            ->fetchAllAssociative();

        /** @var list<CompetitionRank> $ranks */
        $ranks = [];
        /** @var array<string, null> $scoredPlayerSet */
        $scoredPlayerSet = [];
        foreach ($pss as $ps) {
            // player_scoreが同一player_id内ではrow_numの降順でソートされているので
            // 現れたのが2回目以降のplayer_idはより大きいrow_numでスコアが出ているとみなせる
            if (array_key_exists($ps['player_id'], $scoredPlayerSet)) {
                continue;
            }
            $scoredPlayerSet[$ps['player_id']] = null;
            $p = $this->retrievePlayer($tenantDB, $ps['player_id']);

            $ranks[] = new CompetitionRank(
                score: $ps['score'],
                playerID: $p->id,
                playerDisplayName: $p->displayName,
                rowNum: $ps['row_num'],
            );
        }
        usort($ranks, function (CompetitionRank $x, CompetitionRank $y): int {
            if ($x->score === $y->score) {
                return $x->rowNum <=> $y->rowNum;
            }

            return $y->score <=> $x->score;
        });

        /** @var list<CompetitionRank> $pageRanks */
        $pageRanks = [];
        foreach ($ranks as $i => $rank) {
            if ($i < $rankAfter) {
                continue;
            }
            $pageRanks[] = new CompetitionRank(
                rank: $i + 1,
                score: $rank->score,
                playerID: $rank->playerID,
                playerDisplayName: $rank->playerDisplayName,
            );
            if (count($pageRanks) >= 100) {
                break;
            }
        }

        $res = new SuccessResult(
            success: true,
            data: new CompetitionRankingHandlerResult(
                competition: new CompetitionDetail(
                    id: $competition->id,
                    title: $competition->title,
                    isFinished: !is_null($competition->finishedAt),
                ),
                ranks: $pageRanks,
            ),
        );

        $tenantDB->close();
        fclose($fl);

        return $this->jsonResponse($response, $res);
    }

    /**
     * 参加者向けAPI
     * GET /api/player/competitions
     * 大会の一覧を取得する
     */
    public function playerCompetitionsHandler(Request $request, Response $response): Response
    {
        $v = $this->parseViewer($request);

        if ($v->role !== self::ROLE_PLAYER) {
            throw new HttpForbiddenException($request, 'role player required');
        }

        $tenantDB = $this->connectToTenantDB($v->tenantID);

        $this->authorizePlayer($request, $tenantDB, $v->playerID);

        $response = $this->competitionsHandler($response, $v, $tenantDB);

        $tenantDB->close();

        return $response;
    }

    /**
     * テナント管理者向けAPI
     * GET /api/organizer/competitions
     * 大会の一覧を取得する
     */
    public function organizerCompetitionsHandler(Request $request, Response $response): Response
    {
        $v = $this->parseViewer($request);
        if ($v->role !== self::ROLE_ORGANIZER) {
            throw new HttpForbiddenException($request, 'role organizer required');
        }

        $tenantDB = $this->connectToTenantDB($v->tenantID);

        $response = $this->competitionsHandler($response, $v, $tenantDB);

        $tenantDB->close();

        return $response;
    }

    private function competitionsHandler(Response $response, Viewer $v, Connection $tenantDB): Response
    {
        /** @var list<CompetitionDetail> $cds */
        $cds = [];
        $result = $tenantDB->prepare('SELECT * FROM competition WHERE tenant_id=? ORDER BY created_at DESC')
            ->executeQuery([$v->tenantID]);
        while ($row = $result->fetchAssociative()) {
            $cds[] = new CompetitionDetail(
                id: $row['id'],
                title: $row['title'],
                isFinished: !is_null($row['finished_at']),
            );
        }

        $res = new SuccessResult(
            success: true,
            data: new CompetitionsHandlerResult(
                competitions: $cds,
            ),
        );

        return $this->jsonResponse($response, $res);
    }

    /**
     * 共通API
     * GET /api/me
     * JWTで認証した結果、テナントやユーザ情報を返す
     */
    public function meHandler(Request $request, Response $response): Response
    {
        $tenant = $this->retrieveTenantRowFromHeader($request);
        if (is_null($tenant)) {
            throw new RuntimeException('error retrieveTenantRowFromHeader');
        }

        $td = new TenantDetail(
            name: $tenant->name,
            displayName: $tenant->displayName,
        );

        try {
            $v = $this->parseViewer($request);
        } catch (HttpUnauthorizedException) {
            return $this->jsonResponse($response, new SuccessResult(
                success: true,
                data: new MeHandlerResult(
                    tenant: $td,
                    me: null,
                    role: self::ROLE_NONE,
                    loggedIn: false,
                ),
            ));
        }

        if ($v->role === self::ROLE_ADMIN || $v->role === self::ROLE_ORGANIZER) {
            return $this->jsonResponse($response, new SuccessResult(
                success: true,
                data: new MeHandlerResult(
                    tenant: $td,
                    me: null,
                    role: $v->role,
                    loggedIn: true,
                ),
            ));
        }

        $tenantDB = $this->connectToTenantDB($v->tenantID);

        $p = $this->retrievePlayer($tenantDB, $v->playerID);

        if (is_null($p)) {
            return $this->jsonResponse($response, new SuccessResult(
                success: true,
                data: new MeHandlerResult(
                    tenant: $td,
                    me: null,
                    role: self::ROLE_NONE,
                    loggedIn: false,
                ),
            ));
        }

        $tenantDB->close();

        return $this->jsonResponse($response, new SuccessResult(
            success: true,
            data: new MeHandlerResult(
                tenant: $td,
                me: new PlayerDetail(
                    id: $p->id,
                    displayName: $p->displayName,
                    isDisqualified: $p->isDisqualified,
                ),
                role: $v->role,
                loggedIn: false,
            ),
        ));
    }

    /**
     * ベンチマーカー向けAPI
     * POST /initialize
     * ベンチマーカーが起動したときに最初に呼ぶ
     * データベースの初期化などが実行されるため、スキーマを変更した場合などは適宜改変すること
     */
    public function initializeHandler(Request $request, Response $response): Response
    {
        if ($this->execCommand([self::INITIALIZE_SCRIPT], $out) !== 0) {
            throw new RuntimeException(sprintf('error execCommand: %s', $out));
        }

        $res = new InitializeHandlerResult(
            lang: 'php'
        );

        return $this->jsonResponse($response, new SuccessResult(success: true, data: $res));
    }

    private function execCommand(array|string $command, &$out): int
    {
        $fp = fopen('php://temp', 'w+');
        $descriptorSpec = [
            1 => $fp,
            2 => $fp,
        ];

        $process = proc_open($command, $descriptorSpec, $_);
        if ($process === false) {
            throw new RuntimeException('error execCommand: cannot open process');
        }

        $exitCode = proc_close($process);

        rewind($fp);
        $out = stream_get_contents($fp);

        return $exitCode;
    }

    /**
     * @throws UnexpectedValueException
     */
    private function jsonResponse(Response $response, JsonSerializable|array $data, int $statusCode = 200): Response
    {
        $responseBody = json_encode($data, JSON_UNESCAPED_UNICODE);
        if ($responseBody === false) {
            throw new UnexpectedValueException('failed to json_encode');
        }

        $response->getBody()->write($responseBody);

        return $response->withStatus($statusCode)
            ->withHeader('Content-Type', 'application/json; charset=UTF-8');
    }
}
