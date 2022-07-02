<?php

declare(strict_types=1);

namespace SQLTrace;

use JsonSerializable;

final class TraceLog implements JsonSerializable
{
    public function __construct(
        private string $time,
        private string $statement,
        private array $args,
        private float $queryTime,
        private int $affectedRows,
    ) {
    }

    /**
     * @return array{
     *     time: string,
     *     statement: string,
     *     args: array,
     *     query_time: float,
     *     affected_rows: int
     * }
     */
    public function jsonSerialize(): array
    {
        return [
            'time' => $this->time,
            'statement' => $this->statement,
            'args' => $this->args,
            'query_time' => $this->queryTime,
            'affected_rows' => $this->affectedRows,
        ];
    }
}
