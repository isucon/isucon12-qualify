package isucon12.model;

import java.util.Date;

public class PlayerRow {
    private Long tenantId;
    private String id;
    private String displayName;
    private Boolean isDisqualified;
    private Date createdAt;
    private Date updatedAt;

    public PlayerRow(Long tenantId, String id, String displayName, Boolean isDisqualified, Date createdAt, Date updatedAt) {
        super();
        this.tenantId = tenantId;
        this.id = id;
        this.displayName = displayName;
        this.isDisqualified = isDisqualified;
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
    public String getDisplayName() {
        return displayName;
    }
    public void setDisplayName(String displayName) {
        this.displayName = displayName;
    }
    public Boolean getIsDisqualified() {
        return isDisqualified;
    }
    public void setIsDisqualified(Boolean isDisqualified) {
        this.isDisqualified = isDisqualified;
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
