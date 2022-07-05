<?php

declare(strict_types=1);

namespace App\Isuports;

use JsonSerializable;

final class SuccessResult implements JsonSerializable
{
    public function __construct(
        private bool $success,
        private JsonSerializable|array|null $data = null,
    ) {
    }

    /**
     * @return array{status: bool, data?: JsonSerializable|array}
     */
    public function jsonSerialize(): array
    {
        $forJson = ['status' => $this->success];

        if (!is_null($this->data)) {
            $forJson['data'] = $this->data;
        }

        return $forJson;
    }
}

final class FailureResult implements JsonSerializable
{
    public function __construct(
        private bool $success,
        private string $message,
    ) {
    }

    /**
     * @return array{status: bool, message: string}
     */
    public function jsonSerialize(): array
    {
        return [
            'status' => $this->success,
            'message' => $this->message,
        ];
    }
}

final class Viewer
{
    public function __construct(
        public string $role,
        public string $playerID,
        public string $tenantName,
        public int $tenantID,
    ) {
    }
}

final class TenantRow
{
    public function __construct(
        public string $name,
        public string $displayName,
        public int $id = 0,
        public int $createdAt = 0,
        public int $updatedAt = 0,
    ) {
    }

    /**
     * @param array{
     *     id: string,
     *     name: string,
     *     display_name: string,
     *     created_at: int,
     *     updated_at: int,
     * } $row
     */
    public static function fromDB(array $row): self
    {
        return new self(
            id: $row['id'],
            name: $row['name'],
            displayName: $row['display_name'],
            createdAt: $row['created_at'],
            updatedAt: $row['updated_at'],
        );
    }
}

final class PlayerRow
{
    public function __construct(
        public int $tenantID,
        public string $id,
        public string $displayName,
        public bool $isDisqualified,
        public int $createdAt,
        public int $updatedAt,
    ) {
    }

    /**
     * @param array{
     *     tenant_id: int,
     *     id: string,
     *     display_name: string,
     *     is_disqualified: bool,
     *     created_at: int,
     *     updated_at: int,
     * } $row
     */
    public static function fromDB(array $row): self
    {
        return new self(
            tenantID: $row['tenant_id'],
            id: $row['id'],
            displayName: $row['display_name'],
            isDisqualified: $row['is_disqualified'],
            createdAt: $row['created_at'],
            updatedAt: $row['updated_at'],
        );
    }
}

final class CompetitionRow
{
    public function __construct(
        public int $tenantID,
        public string $id,
        public string $title,
        public ?int $finishedAt,
        public int $createdAt,
        public int $updatedAt,
    ) {
    }

    /**
     * @param array{
     *     tenant_id: int,
     *     id: string,
     *     title: string,
     *     finished_at: ?int,
     *     created_at: int,
     *     updated_at: int,
     * } $row
     */
    public static function fromDB(array $row): self
    {
        return new self(
            tenantID: $row['tenant_id'],
            id: $row['id'],
            title: $row['title'],
            finishedAt: $row['finished_at'],
            createdAt: $row['created_at'],
            updatedAt: $row['updated_at'],
        );
    }
}

final class PlayerScoreRow
{
    public function __construct(
        public int $tenantID,
        public string $id,
        public string $playerID,
        public string $competitionID,
        public int $score,
        public int $rowNum,
        public int $createdAt,
        public int $updatedAt,
    ) {
    }

    /**
     * @param array{
     *     tenant_id: int,
     *     id: string,
     *     player_id: string,
     *     competition_id: string,
     *     score: int,
     *     row_num: int,
     *     created_at: int,
     *     updated_at: int,
     * } $row
     */
    public static function fromDB(array $row): self
    {
        return new self(
            tenantID: $row['tenant_id'],
            id: $row['id'],
            playerID: $row['player_id'],
            competitionID: $row['competition_id'],
            score: $row['score'],
            rowNum: $row['row_num'],
            createdAt: $row['created_at'],
            updatedAt: $row['updated_at'],
        );
    }
}

final class TenantDetail implements JsonSerializable
{
    public function __construct(
        private string $name,
        private string $displayName,
    ) {
    }

    /**
     * @return array{name: string, display_name: string}
     */
    public function jsonSerialize(): array
    {
        return [
            'name' => $this->name,
            'display_name' => $this->displayName,
        ];
    }
}

final class TenantsAddHandlerResult implements JsonSerializable
{
    public function __construct(private TenantDetail $tenant)
    {
    }

    /**
     * @return array{tenant: TenantDetail}
     */
    public function jsonSerialize(): array
    {
        return ['tenant' => $this->tenant];
    }
}

final class BillingReport implements JsonSerializable
{
    public function __construct(
        private string $competitionID,
        private string $competitionTitle,
        private int $playerCount,
        private int $visitorCount,
        private int $billingPlayerYen,
        private int $billingVisitorYen,
        private int $billingYen,
    ) {
    }

    /**
     * @return array{
     *     competition_id: string,
     *     competition_title: string,
     *     player_count: int,
     *     visitor_count: int,
     *     billing_player_yen: int,
     *     billing_visitor_yen: int,
     *     billing_yen: int,
     * }
     */
    public function jsonSerialize(): array
    {
        return [
            'competition_id' => $this->competitionID,
            'competition_title' => $this->competitionTitle,
            'player_count' => $this->playerCount,
            'visitor_count' => $this->visitorCount,
            'billing_player_yen' => $this->billingPlayerYen,
            'billing_visitor_yen' => $this->billingVisitorYen,
            'billing_yen' => $this->billingYen,
        ];
    }
}

final class TenantWithBilling implements JsonSerializable
{
    public function __construct(
        private string $id,
        private string $name,
        private string $displayName,
        private int $billingYen,
    ) {
    }

    /**
     * @return array{
     *     id: string,
     *     name: string,
     *     display_name: string,
     *     billing: int,
     * }
     */
    public function jsonSerialize(): array
    {
        return [
            'id' => $this->id,
            'name' => $this->name,
            'display_name' => $this->displayName,
            'billing' => $this->billingYen,
        ];
    }
}

final class TenantsBillingHandlerResult implements JsonSerializable
{
    /**
     * @param list<TenantWithBilling> $tenants
     */
    public function __construct(private array $tenants)
    {
    }

    /**
     * @return array{tenants: list<TenantWithBilling>}
     */
    public function jsonSerialize(): mixed
    {
        return ['tenants' => $this->tenants];
    }
}

final class PlayerDetail implements JsonSerializable
{
    public function __construct(
        private string $id,
        private string $displayName,
        private bool $isDisqualified,
    ) {
    }

    /**
     * @return array{id: string, display_name: string, is_disqualified: bool}
     */
    public function jsonSerialize(): array
    {
        return [
            'id' => $this->id,
            'display_name' => $this->displayName,
            'is_disqualified' => $this->isDisqualified,
        ];
    }
}

final class PlayersListHandlerResult implements JsonSerializable
{
    /**
     * @param list<PlayerDetail> $players
     */
    public function __construct(private array $players)
    {
    }

    /**
     * @return array{players: list<PlayerDetail>}
     */
    public function jsonSerialize(): array
    {
        return ['players' => $this->players];
    }
}

final class PlayersAddHandlerResult implements JsonSerializable
{
    /**
     * @param list<PlayerDetail> $players
     */
    public function __construct(private array $players)
    {
    }

    /**
     * @return array{players: list<PlayerDetail>}
     */
    public function jsonSerialize(): array
    {
        return ['players' => $this->players];
    }
}

final class PlayerDisqualifiedHandlerResult implements JsonSerializable
{
    public function __construct(private PlayerDetail $player)
    {
    }

    /**
     * @return array{player: PlayerDetail}
     */
    public function jsonSerialize(): array
    {
        return ['player' => $this->player];
    }
}

final class CompetitionDetail implements JsonSerializable
{
    public function __construct(
        private string $id,
        private string $title,
        private bool $isFinished,
    ) {
    }

    /**
     * @return array{id: string, title: string, is_finished: bool}
     */
    public function jsonSerialize(): array
    {
        return [
            'id' => $this->id,
            'title' => $this->title,
            'is_finished' => $this->isFinished,
        ];
    }
}

final class CompetitionsAddHandlerResult implements JsonSerializable
{
    public function __construct(private CompetitionDetail $competition)
    {
    }

    /**
     * @return array{competition: CompetitionDetail}
     */
    public function jsonSerialize(): array
    {
        return ['competition' => $this->competition];
    }
}

final class ScoreHandlerResult implements JsonSerializable
{
    public function __construct(private int $rows)
    {
    }

    /**
     * @return array{rows: int}
     */
    public function jsonSerialize(): array
    {
        return ['rows' => $this->rows];
    }
}

final class BillingHandlerResult implements JsonSerializable
{
    /**
     * @param list<BillingReport> $reports
     */
    public function __construct(private array $reports)
    {
    }

    /**
     * @return array{reports: list<BillingReport>}
     */
    public function jsonSerialize(): array
    {
        return ['reports' => $this->reports];
    }
}

final class PlayerScoreDetail implements JsonSerializable
{
    public function __construct(
        private string $competitionTitle,
        private int $score,
    ) {
    }

    /**
     * @return array{competition_title: string, score: int}
     */
    public function jsonSerialize(): array
    {
        return [
            'competition_title' => $this->competitionTitle,
            'score' => $this->score,
        ];
    }
}

final class PlayerHandlerResult implements JsonSerializable
{
    /**
     * @param list<PlayerScoreDetail> $scores
     */
    public function __construct(
        private PlayerDetail $player,
        private array $scores,
    ) {
    }

    /**
     * @return array{player: PlayerDetail, scores: list<PlayerScoreDetail>}
     */
    public function jsonSerialize(): array
    {
        return [
            'player' => $this->player,
            'scores' => $this->scores,
        ];
    }
}

final class CompetitionRank implements JsonSerializable
{
    public function __construct(
        private int $rank,
        private int $score,
        private string $playerID,
        private string $playerDisplayName,
        public int $rowNum, // APIレスポンスのJSONには含まれない
    ) {
    }

    /**
     * @return array{
     *     rank: int,
     *     score: int,
     *     player_id: string,
     *     player_display_name: string,
     * }
     */
    public function jsonSerialize(): array
    {
        return [
            'rank' => $this->rank,
            'score' => $this->score,
            'player_id' => $this->playerID,
            'player_display_name' => $this->playerDisplayName,
        ];
    }
}

final class CompetitionRankingHandlerResult implements JsonSerializable
{
    /**
     * @param list<CompetitionRank> $ranks
     */
    public function __construct(
        private CompetitionDetail $competition,
        private array $ranks,
    ) {
    }

    /**
     * @return array{
     *     competition: CompetitionDetail,
     *     ranks: list<CompetitionRank>,
     * }
     */
    public function jsonSerialize(): array
    {
        return [
            'competition' => $this->competition,
            'ranks' => $this->ranks,
        ];
    }
}

final class CompetitionsHandlerResult implements JsonSerializable
{
    /**
     * @param list<CompetitionDetail> $competitions
     */
    public function __construct(private array $competitions)
    {
    }

    /**
     * @return array{competitions: list<CompetitionDetail>}
     */
    public function jsonSerialize(): array
    {
        return ['competitions' => $this->competitions];
    }
}

final class MeHandlerResult implements JsonSerializable
{
    public function __construct(
        private TenantDetail $tenant,
        private ?PlayerDetail $me,
        private string $role,
        private bool $loggedIn,
    ) {
    }

    /**
     * @return array{
     *     tenant: TenantDetail,
     *     me: ?PlayerDetail,
     *     role: string,
     *     logged_in: bool
     * }
     */
    public function jsonSerialize(): array
    {
        return [
            'tenant' => $this->tenant,
            'me' => $this->me,
            'role' => $this->role,
            'logged_in' => $this->loggedIn,
        ];
    }
}

final class InitializeHandlerResult implements JsonSerializable
{
    public function __construct(
        private string $lang,
        private string $appeal,
    ) {
    }

    /**
     * @return array{lang: string, appeal: string}
     */
    public function jsonSerialize(): array
    {
        return [
            'lang' => $this->lang,
            'appeal' => $this->appeal,
        ];
    }
}
