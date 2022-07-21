<?php

declare(strict_types=1);

namespace App\Application\Handlers;

use App\Isuports\FailureResult;
use Psr\Http\Message\ResponseInterface as Response;
use Slim\Exception\HttpException;
use Slim\Handlers\ErrorHandler as SlimErrorHandler;

class HttpErrorHandler extends SlimErrorHandler
{
    /**
     * @inheritdoc
     */
    protected function respond(): Response
    {
        $statusCode = 500;
        if ($this->exception instanceof HttpException) {
            $statusCode = $this->exception->getCode();
        }

        $message = '';
        if ($this->displayErrorDetails) {
            $message = $this->exception->getMessage();
        }
        $payload = json_encode(new FailureResult(
            success: false,
            message: $message,
        ), JSON_UNESCAPED_UNICODE);

        $response = $this->responseFactory->createResponse($statusCode);
        $response->getBody()->write($payload);

        return $response->withHeader('Content-Type', 'application/json; charset=UTF-8');
    }
}
