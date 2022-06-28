<?php

declare(strict_types=1);

namespace App\Isuports;

use JsonSerializable;
use Doctrine\DBAL\Connection;
use Doctrine\DBAL\DriverManager;
use Doctrine\DBAL\Exception as DBException;
use PDO;
use Psr\Http\Message\ResponseInterface as Response;
use Psr\Http\Message\ServerRequestInterface as Request;
use RuntimeException;
use UnexpectedValueException;

final class Handlers
{
    public function __construct(
        private Connection $adminDB,
    ) {
    }

    /**
     * テナントDBのパスを返す
     */
    private function tenantDBPath(int $id): string
    {
        $tenantDBDir = getenv('ISUCON_TENENT_DB_DIR') ?: __DIR__ . '/../../../tenant_db';

        return $tenantDBDir . DIRECTORY_SEPARATOR . sprintf('%d.db', $id);
    }

    /**
     * テナントDBに接続する
     *
     * @throws RuntimeException
     */
    private function connectToTenantDB(int $id): Connection
    {
        try {
            return DriverManager::getConnection([
                'path' => $this->tenantDBPath($id),
                'driver' => 'pdo_sqlite',
                'driverOptions' => [
                    PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION,
                    PDO::ATTR_DEFAULT_FETCH_MODE => PDO::FETCH_ASSOC,
                ],
            ]);
        } catch (DBException $e) {
            throw new RuntimeException(message: 'failed to open tenant DB: ' . $e->getMessage(), previous: $e);
        }
    }

    public function meHandler(Request $request, Response $response): Response
    {
        // TODO: 仮実装
        $td = new TenantDetail(
            name: 'test',
            displayName: 'テスト',
        );

        return $this->jsonResponse($response, new SuccessResult(
            success: true,
            data: new MeHandlerResult(
                tenant: $td,
                me: null,
                role: Role::ofNone(),
                loggedIn: false,
            ),
        ));
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
