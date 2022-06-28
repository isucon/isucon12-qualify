<?php

declare(strict_types=1);

namespace App\Isuports;

use JsonSerializable;
use PDO;
use PDOException;
use Psr\Http\Message\ResponseInterface as Response;
use Psr\Http\Message\ServerRequestInterface as Request;
use RuntimeException;
use UnexpectedValueException;

final class Handlers
{
    public function __construct(
        private PDO $adminDB,
    ) {
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
