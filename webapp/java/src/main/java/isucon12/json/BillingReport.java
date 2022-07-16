package isucon12.json;

public class BillingReport {
    private String competitionId;
    private String competitionTitle;
    private Long playerCount; // スコアを登録した参加者数
    private Long visitorCount; // ランキングを閲覧だけした(スコアを登録していない)参加者数
    private Long billingPlayerYenjson; // 請求金額 スコアを登録した参加者分
    private Long billingVisitorYen; // 請求金額 ランキングを閲覧だけした(スコアを登録していない)参加者分
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

    public Long getBillingPlayerYenjson() {
        return billingPlayerYenjson;
    }

    public void setBillingPlayerYenjson(Long billingPlayerYenjson) {
        this.billingPlayerYenjson = billingPlayerYenjson;
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
