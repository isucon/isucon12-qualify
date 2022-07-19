<?php

declare(strict_types=1);

namespace SQLTrace;

use Doctrine\DBAL\Driver\Connection as ConnectionInterface;
use Doctrine\DBAL\Driver\Middleware\AbstractConnectionMiddleware;
use Doctrine\DBAL\Driver\Result;
use Doctrine\DBAL\Driver\Statement as StatementInterface;
use Psr\Log\LoggerInterface;

/**
 * @see {\Doctrine\DBAL\Logging\Connection}
 */
final class Connection extends AbstractConnectionMiddleware implements ConnectionInterface
{
    public function __construct(
        ConnectionInterface $connection,
        private LoggerInterface $logger,
    ) {
        parent::__construct($connection);
    }

    public function prepare(string $sql): StatementInterface
    {
        return new Statement(
            parent::prepare($sql),
            $this->logger,
            $sql,
        );
    }

    public function query(string $sql): Result
    {
        $time = date('Y-m-d\TH:i:sP');
        $start = microtime(true);

        $result = parent::query($sql);

        $traceLog = new TraceLog(
            time: $time,
            statement: $sql,
            args: [],
            queryTime: (microtime(true) - $start) * 1000,
            affectedRows: $result->rowCount(),
        );

        $this->logger->debug(json_encode($traceLog, JSON_UNESCAPED_UNICODE));

        return $result;
    }

    public function exec(string $sql): int
    {
        $time = date('Y-m-d\TH:i:sP');
        $start = microtime(true);

        $rowCount = parent::exec($sql);

        $traceLog = new TraceLog(
            time: $time,
            statement: $sql,
            args: [],
            queryTime: (microtime(true) - $start) * 1000,
            affectedRows: $rowCount,
        );

        $this->logger->debug(json_encode($traceLog, JSON_UNESCAPED_UNICODE));

        return $rowCount;
    }
}
