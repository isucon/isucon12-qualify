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
use RuntimeException;
use Slim\Exception\HttpBadRequestException;
use Slim\Exception\HttpForbiddenException;
use Slim\Exception\HttpNotFoundException;
use Slim\Exception\HttpUnauthorizedException;
use UnexpectedValueException;

final class Handlers
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
     * テナントDBを新規に作成する
     */
    private function createTenantDB(int $id): void
    {
        $p = $this->tenantDBPath($id);

        $process = proc_open(
            ['sh', '-c', sprintf('sqlite3 %s < %s', $p, self::TENANT_DB_SCHEMA_FILE_PATH)],
            [
                0 => ['pipe', 'r'],
                1 => ['pipe', 'w'],
                2 => ['pipe', 'w'],
            ],
            $pipes,
        );

        if ($process === false) {
            throw new RuntimeException(
                vsprintf(
                    'failed to exec sqlite3 %s < %s: cannot open process',
                    [$p, self::TENANT_DB_SCHEMA_FILE_PATH],
                ),
            );
        }

        fclose($pipes[0]);
        $out = stream_get_contents($pipes[1]);
        fclose($pipes[1]);
        if (proc_close($process) !== 0) {
            throw new RuntimeException(
                vsprintf(
                    'failed to exec sqlite3 %s < %s, out=%s',
                    [$p, self::TENANT_DB_SCHEMA_FILE_PATH, $out],
                ),
            );
        }
    }

    /**
     * システム全体で一意なIDを生成する
     */
    private function dispenseID(): string
    {
        $id = 0;
        /** @var ?RuntimeException $lastErr */
        $lastErr = null;
        for ($i = 0; $i < 100; $i++) {
            try {
                $this->adminDB->prepare('REPLACE INTO id_generator (stub) VALUES (?);')
                    ->executeStatement(['a']);
            } catch (DBException $e) {
                if ($e->getCode() === 1213) { // deadlock
                    $lastErr = new RuntimeException(
                        sprintf('error REPLACE INTO id_generator: %s', $e->getMessage()),
                        previous: $e
                    );
                    continue;
                }
                throw new RuntimeException(sprintf('error REPLACE INTO id_generator: %s', $e->getMessage()));
            }

            try {
                $id = $this->adminDB->lastInsertId();
            } catch (DBException $e) {
                throw new RuntimeException(sprintf('error ret.LastInsertId: %s', $e->getMessage()), previous: $e);
            }
            break;
        }

        if ($id !== 0) {
            return (string)$id;
        }

        throw $lastErr;
    }

    /**
     * テナントDBに接続する
     *
     * @throws RuntimeException
     */
    private function connectToTenantDB(int $id): Connection
    {
        try {
            return DriverManager::getConnection(
                params: [
                    'path' => $this->tenantDBPath($id),
                    'driver' => 'pdo_sqlite',
                    'driverOptions' => [
                        PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION,
                        PDO::ATTR_DEFAULT_FETCH_MODE => PDO::FETCH_ASSOC,
                    ],
                ],
                config: $this->sqliteConfiguration,
            );
        } catch (DBException $e) {
            throw new RuntimeException(message: 'failed to open tenant DB: ' . $e->getMessage(), previous: $e);
        }
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

        $keyFilename = getenv('ISUCON_JWT_KEY_FILE') ?: __DIR__ . '/../../../go/public.pem';
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
            throw new HttpUnauthorizedException(
                $request,
                sprintf('invalid token: subject is not found in token: %s', $tokenStr),
            );
        }

        if (!property_exists($token, 'role')) {
            throw new HttpUnauthorizedException(
                $request,
                sprintf('invalid token: role is not found in token: %s', $tokenStr),
            );
        }

        /** @var string $role */
        $role = match ($token->role) {
            self::ROLE_ADMIN, self::ROLE_ORGANIZER, self::ROLE_PLAYER => $token->role,
            default => new HttpUnauthorizedException(
                $request,
                sprintf('invalid token: %s is invalid role: %s', $token->role, $tokenStr),
            ),
        };

        /** @var list<string> $aud */
        $aud = $token->aud;
        if (count($aud) !== 1) {
            throw new HttpUnauthorizedException(
                $request,
                sprintf('invalid token: aud field is few or too much: %s', $tokenStr),
            );
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
                sprintf(
                    'invalid token: tenant name is not match with %s: %s',
                    $request->getHeader('Host')[0],
                    $tokenStr
                ),
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
        $tenantName = preg_replace(
            '/' . preg_quote($baseHost) . '$/',
            '',
            $request->getHeader('Host')[0]
        );

        // SaaS管理者用ドメイン
        if ($tenantName === 'admin') {
            return new TenantRow(
                name:'admin',
                displayName: 'admin'
            );
        }

        // テナントの存在確認
        try {
            $row = $this->adminDB->prepare('SELECT * FROM tenant WHERE name = ?')
                ->executeQuery([$tenantName])
                ->fetchAssociative();
        } catch (DBException $e) {
            throw new RuntimeException(
                sprintf('failed to Select tenant: name=%s, %s', $tenantName, $e->getMessage()),
                previous: $e,
            );
        }

        if ($row === false) {
            return null;
        }

        return TenantRow::fromDB($row);
    }

    /**
     * 参加者を取得する
     *
     * @throws RuntimeException
     */
    private function retrievePlayer(Connection $tenantDB, string $id): ?PlayerRow
    {
        try {
            $row = $tenantDB->prepare('SELECT * FROM player WHERE id = ?')
                ->executeQuery([$id])
                ->fetchAssociative();
        } catch (DBException $e) {
            throw new RuntimeException(
                sprintf('error Select player: id=%s, %s', $id, $e->getMessage()),
                previous: $e,
            );
        }

        if ($row === false) {
            return null;
        }

        return PlayerRow::fromDB($row);
    }

    /**
     * 参加者を認可する
     * 参加者向けAPIで呼ばれる
     */
    private function authorizePlayer(Connection $tenantDB, string $id): void
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    /**
     * 大会を取得する
     */
    private function retrieveCompetition(Connection $tenantDB, string $id): CompetitionRow
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
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
     * @throws RuntimeException
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
            throw new HttpNotFoundException(
                $request,
                sprintf('%s has not this API', $v->tenantName),
            );
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
            $this->adminDB->prepare(
                'INSERT INTO tenant (name, display_name, created_at, updated_at) VALUES (?, ?, ?, ?)'
            )->executeStatement([$name, $displayName, $now, $now]);
        } catch (DBException $e) {
            if ($e->getCode() === 1062) { // duplicate entry
                throw new HttpBadRequestException($request, 'duplicate tenant');
            }

            throw new RuntimeException(
                vsprintf(
                    'error Insert tenant: name=%s, displayName=%s, createdAt=%d, updatedAt=%d, %s',
                    [$name, $displayName, $now, $now, $e->getMessage()],
                ),
                previous: $e,
            );
        }

        try {
            $id = (int)$this->adminDB->lastInsertId();
        } catch (DBException $e) {
            throw new RuntimeException(sprintf('error get LastInsertId: %s', $e->getMessage()));
        }

        $this->createTenantDB($id);

        $res = new TenantsAddHandlerResult(
            tenant: new TenantDetail(
                name: $name,
                displayName: $displayName,
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
    private function billingReportByCompetition(
        Connection $tenantDB,
        int $tenantID,
        string $competitionID,
    ): BillingReport {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    /**
     * SaaS管理者用API
     * テナントごとの課金レポートを最大20件、テナントのid降順で取得する
     * POST /api/admin/tenants/billing
     * URL引数beforeを指定した場合、指定した値よりもidが小さいテナントの課金レポートを取得する
     */
    public function tenantsBillingHandler(Request $request, Response $response): Response
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    /**
     * テナント管理者向けAPI
     * GET /api/organizer/players
     * 参加者一覧を返す
     */
    public function playersListHandler(Request $request, Response $response): Response
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
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
            throw new RuntimeException('error $request->getParsedBody()');
        }

        /** @var list<string> $displayNames */
        $displayNames = $params['display_name'] ?? [];

        /** @var list<PlayerDetail> $pds */
        $pds = [];
        foreach ($displayNames as $displayName) {
            $id = $this->dispenseID();

            $now = time();
            try {
                $tenantDB->prepare('INSERT INTO player (id, tenant_id, display_name, is_disqualified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)')
                    ->executeStatement([$id, $v->tenantID, $displayName, false, $now, $now]);
            } catch (DBException $e) {
                throw new RuntimeException(
                    vsprintf(
                        'error Insert player at tenantDB: id=%s, displayName=%s, isDisqualified=%s, createdAt=%d, updatedAt=%d, %s',
                        [$id, $displayName, false, $now, $now, $e->getMessage()],
                    ),
                    previous: $e,
                );
            }

            $p = $this->retrievePlayer($tenantDB, $id);

            $pds[] = new PlayerDetail(
                id: $p->id,
                displayName: $p->displayName,
                isDisqualified: $p->isDisqualified,
            );
        }

        $tenantDB->close();

        $res = new PlayersAddHandlerResult(players: $pds);

        return $this->jsonResponse($response, new SuccessResult(success: true, data: $res));
    }

    /**
     * テナント管理者向けAPI
     * POST /api/organizer/player/:player_id/disqualified
     * 参加者を失格にする
     */
    public function playerDisqualifiedHandler(Request $request, Response $response): Response
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    /**
     * テナント管理者向けAPI
     * POST /api/organizer/competitions/add
     * 大会を追加する
     */
    public function competitionsAddHandler(Request $request, Response $response): Response
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    /**
     * テナント管理者向けAPI
     * POST /api/organizer/competition/:competition_id/finish
     * 大会を終了する
     */
    public function competitionFinishHandler(Request $request, Response $response): Response
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    /**
     * テナント管理者向けAPI
     * POST /api/organizer/competition/:competition_id/score
     * 大会のスコアをCSVでアップロードする
     */
    public function competitionScoreHandler(Request $request, Response $response): Response
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    /**
     * テナント管理者向けAPI
     * GET /api/organizer/billing
     * テナント内の課金レポートを取得する
     */
    public function billingHandler(Request $request, Response $response): Response
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    /**
     * 参加者向けAPI
     * GET /api/player/player/:player_id
     * 参加者の詳細情報を取得する
     */
    public function playerHandler(Request $request, Response $response): Response
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    /**
     * 参加者向けAPI
     * GET /api/player/competition/:competition_id/ranking
     * 大会ごとのランキングを取得する
     */
    public function competitionRankingHandler(Request $request, Response $response): Response
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    /**
     * 参加者向けAPI
     * GET /api/player/competitions
     * 大会の一覧を取得する
     */
    public function playerCompetitionsHandler(Request $request, Response $response): Response
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    /**
     * 主催者向けAPI
     * GET /api/organizer/competitions
     * 大会の一覧を取得する
     */
    public function organizerCompetitionsHandler(Request $request, Response $response): Response
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    private function competitionsHandler(Response $response, Viewer $v, Connection $tenantDB): Response
    {
        // TODO: 実装
        throw new \LogicException('not implemented');
    }

    /**
     * 共通API
     * GET /api/me
     * JWTで認証した結果、テナントやユーザ情報を返す
     */
    public function meHandler(Request $request, Response $response): Response
    {
        $tenant = $this->retrieveTenantRowFromHeader($request);

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

        $tenantDB->close();

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
        $process = proc_open(
            [self::INITIALIZE_SCRIPT],
            [
                0 => ['pipe', 'r'],
                1 => ['pipe', 'w'],
                2 => ['pipe', 'w'],
            ],
            $pipes,
        );

        if ($process === false) {
            throw new RuntimeException('error exec: cannot open process');
        }

        fclose($pipes[0]);
        $out = stream_get_contents($pipes[1]);
        fclose($pipes[1]);
        if (proc_close($process) !== 0) {
            throw new RuntimeException(sprintf('error exec: %s', $out));
        }

        $res = new InitializeHandlerResult(
            lang: 'php',
            // 頑張ったポイントやこだわりポイントがあれば書いてください
            // 競技中の最後に計測したものを参照して、講評記事などで使わせていただきます
            appeal: '',
        );

        return $this->jsonResponse($response, new SuccessResult(success: true, data: $res));
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
