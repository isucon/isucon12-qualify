package isucon12.model;

import java.util.Date;

public class PlayerScoreRow {
    private Long tenantId;
    private String id;
    private String playerId;
    private String competitionId;
    private Long score;
    private Long rowNum;
    private Date createdAt;
    private Date updatedAt;

    public PlayerScoreRow(Long tenantId, String id, String playerId, String competitionId, Long score, Long rowNum, Date createdAt, Date updatedAt) {
        super();
        this.tenantId = tenantId;
        this.id = id;
        this.playerId = playerId;
        this.competitionId = competitionId;
        this.score = score;
        this.rowNum = rowNum;
        this.createdAt = createdAt;
        this.updatedAt = updatedAt;
    }

    public Long getTenantId() {
        return tenantId;
    }

    public void setTenantId(Long tenantId) {
        this.tenantId = tenantId;
    }

    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    public String getPlayerId() {
        return playerId;
    }

    public void setPlayerId(String playerId) {
        this.playerId = playerId;
    }

    public String getCompetitionId() {
        return competitionId;
    }

    public void setCompetitionId(String competitionId) {
        this.competitionId = competitionId;
    }

    public Long getScore() {
        return score;
    }

    public void setScore(Long score) {
        this.score = score;
    }

    public Long getRowNum() {
        return rowNum;
    }

    public void setRowNum(Long rowNum) {
        this.rowNum = rowNum;
    }

    public Date getCreatedAt() {
        return createdAt;
    }

    public void setCreatedAt(Date createdAt) {
        this.createdAt = createdAt;
    }

    public Date getUpdatedAt() {
        return updatedAt;
    }

    public void setUpdatedAt(Date updatedAt) {
        this.updatedAt = updatedAt;
    }
}
