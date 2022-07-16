package isucon12.json;

import com.fasterxml.jackson.annotation.JsonProperty;

public class BillingReport {
    @JsonProperty("competition_id")
    private String competitionId;
    @JsonProperty("competition_title")
    private String competitionTitle;
    @JsonProperty("player_count")
    private Long playerCount; // スコアを登録した参加者数
    @JsonProperty("visitor_count")
    private Long visitorCount; // ランキングを閲覧だけした(スコアを登録していない)参加者数
    @JsonProperty("billing_player_yen")
    private Long billingPlayerYen; // 請求金額 スコアを登録した参加者分
    @JsonProperty("billing_visitor_yen")
    private Long billingVisitorYen; // 請求金額 ランキングを閲覧だけした(スコアを登録していない)参加者分
    @JsonProperty("billing_yen")
    private Long billingYen; // 合計請求金額

    public String getCompetitionId() {
        return competitionId;
    }

    public void setCompetitionId(String competitionId) {
        this.competitionId = competitionId;
    }

    public String getCompetitionTitle() {
        return competitionTitle;
    }

    public void setCompetitionTitle(String competitionTitle) {
        this.competitionTitle = competitionTitle;
    }

    public Long getPlayerCount() {
        return playerCount;
    }

    public void setPlayerCount(Long playerCount) {
        this.playerCount = playerCount;
    }

    public Long getVisitorCount() {
        return visitorCount;
    }

    public void setVisitorCount(Long visitorCount) {
        this.visitorCount = visitorCount;
    }

    public Long getBillingPlayerYen() {
        return billingPlayerYen;
    }

    public void setBillingPlayerYen(Long billingPlayerYen) {
        this.billingPlayerYen = billingPlayerYen;
    }

    public Long getBillingVisitorYen() {
        return billingVisitorYen;
    }

    public void setBillingVisitorYen(Long billingVisitorYen) {
        this.billingVisitorYen = billingVisitorYen;
    }

    public Long getBillingYen() {
        return billingYen;
    }

    public void setBillingYen(Long billingYen) {
        this.billingYen = billingYen;
    }
}
