<?php

declare(strict_types=1);

namespace App\Isuports;

use JsonSerializable;

class SuccessResult implements JsonSerializable
{
    public function __construct(
        public bool $success,
        public JsonSerializable|array|null $data = null,
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

class FailureResult implements JsonSerializable
{
    public function __construct(
        public bool $success,
        public string $message,
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

/**
 * アクセスしてきた人の情報
 */
class Viewer
{
    public function __construct(
        public string $role,
        public string $playerID,
        public string $tenantName,
        public ?int $tenantID,
    ) {
    }
}

class TenantRow
{
    public function __construct(
        public string $name,
        public string $displayName,
        public ?int $id = null,
        public ?int $createdAt = null,
        public ?int $updatedAt = null,
    ) {
    }
}

class PlayerRow
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
}

class CompetitionRow
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
}

class TenantsAddHandlerResult implements JsonSerializable
{
    public function __construct(public TenantWithBilling $tenant)
    {
    }

    /**
     * @return array{tenant: TenantWithBilling}
     */
    public function jsonSerialize(): array
    {
        return ['tenant' => $this->tenant];
    }
}

class BillingReport implements JsonSerializable
{
    public function __construct(
        public string $competitionID,
        public string $competitionTitle,
        public int $playerCount,
        public int $visitorCount,
        public int $billingPlayerYen,
        public int $billingVisitorYen,
        public int $billingYen,
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

class TenantWithBilling implements JsonSerializable
{
    public function __construct(
        public string $id,
        public string $name,
        public string $displayName,
        public int $billingYen = 0,
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

class TenantsBillingHandlerResult implements JsonSerializable
{
    /**
     * @param list<TenantWithBilling> $tenants
     */
    public function __construct(public array $tenants)
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

class PlayerDetail implements JsonSerializable
{
    public function __construct(
        public string $id,
        public string $displayName,
        public bool $isDisqualified,
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

class PlayersListHandlerResult implements JsonSerializable
{
    /**
     * @param list<PlayerDetail> $players
     */
    public function __construct(public array $players)
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

class PlayersAddHandlerResult implements JsonSerializable
{
    /**
     * @param list<PlayerDetail> $players
     */
    public function __construct(public array $players)
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

class PlayerDisqualifiedHandlerResult implements JsonSerializable
{
    public function __construct(public PlayerDetail $player)
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

class CompetitionDetail implements JsonSerializable
{
    public function __construct(
        public string $id,
        public string $title,
        public bool $isFinished,
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

class CompetitionsAddHandlerResult implements JsonSerializable
{
    public function __construct(public CompetitionDetail $competition)
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

class ScoreHandlerResult implements JsonSerializable
{
    public function __construct(public int $rows)
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

class BillingHandlerResult implements JsonSerializable
{
    /**
     * @param list<BillingReport> $reports
     */
    public function __construct(public array $reports)
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

class PlayerScoreDetail implements JsonSerializable
{
    public function __construct(
        public string $competitionTitle,
        public int $score,
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

class PlayerHandlerResult implements JsonSerializable
{
    /**
     * @param list<PlayerScoreDetail> $scores
     */
    public function __construct(
        public PlayerDetail $player,
        public array $scores,
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

class CompetitionRank implements JsonSerializable
{
    public function __construct(
        public int $score,
        public string $playerID,
        public string $playerDisplayName,
        public ?int $rank = null,
        public ?int $rowNum = null, // APIレスポンスのJSONには含まれない
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

class CompetitionRankingHandlerResult implements JsonSerializable
{
    /**
     * @param list<CompetitionRank> $ranks
     */
    public function __construct(
        public CompetitionDetail $competition,
        public array $ranks,
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

class CompetitionsHandlerResult implements JsonSerializable
{
    /**
     * @param list<CompetitionDetail> $competitions
     */
    public function __construct(public array $competitions)
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

class TenantDetail implements JsonSerializable
{
    public function __construct(
        public string $name,
        public string $displayName,
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

class MeHandlerResult implements JsonSerializable
{
    public function __construct(
        public TenantDetail $tenant,
        public ?PlayerDetail $me,
        public string $role,
        public bool $loggedIn,
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

class InitializeHandlerResult implements JsonSerializable
{
    public function __construct(public string $lang)
    {
    }

    /**
     * @return array{lang: string}
     */
    public function jsonSerialize(): array
    {
        return [
            'lang' => $this->lang,
        ];
    }
}
