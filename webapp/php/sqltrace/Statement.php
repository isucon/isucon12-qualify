<?php

declare(strict_types=1);

namespace SQLTrace;

use Doctrine\DBAL\Driver\Middleware\AbstractStatementMiddleware;
use Doctrine\DBAL\Driver\Result;
use Doctrine\DBAL\Driver\Statement as StatementInterface;
use Doctrine\DBAL\ParameterType;
use Psr\Log\LoggerInterface;

/**
 * @see {\Doctrine\DBAL\Logging\Statement}
 */
final class Statement extends AbstractStatementMiddleware implements StatementInterface
{
    /** @var array<int,mixed>|array<string,mixed> */
    private array $params = [];

    public function __construct(
        StatementInterface $statement,
        private LoggerInterface $logger,
        private string $sql,
    ) {
        parent::__construct($statement);
    }

    public function bindParam($param, &$variable, $type = ParameterType::STRING, $length = null): bool
    {
        $this->params[$param] = &$variable;

        return parent::bindParam($param, $variable, $type, ...array_slice(func_get_args(), 3));
    }

    public function bindValue($param, $value, $type = ParameterType::STRING): bool
    {
        $this->params[$param] = $value;

        return parent::bindValue($param, $value, $type);
    }

    public function execute($params = null): Result
    {
        $time = date('Y-m-d\TH:i:sP');
        $start = microtime(true);

        $result = parent::execute($params);

        $traceLog = new TraceLog(
            time: $time,
            statement: $this->sql,
            args: $params ?? $this->params,
            queryTime: (microtime(true) - $start) * 1000,
            affectedRows: $result->rowCount(),
        );

        $this->logger->debug(json_encode($traceLog, JSON_UNESCAPED_UNICODE));

        return $result;
    }
}
