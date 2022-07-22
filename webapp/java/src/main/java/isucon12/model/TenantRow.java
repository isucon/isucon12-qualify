package isucon12.model;

import java.util.Date;

public class TenantRow {
    private Long id;
    private String name;
    private String displayName;
    private Date createdAt;
    private Date updatedAt;

    public TenantRow() {
    }

    public TenantRow(String name, String displayName) {
        this.name = name;
        this.displayName = displayName;
    }

    public Long getId() {
        return id;
    }
    public void setId(Long id) {
        this.id = id;
    }
    public String getName() {
        return name;
    }
    public void setName(String name) {
        this.name = name;
    }
    public String getDisplayName() {
        return displayName;
    }
    public void setDisplayName(String displayName) {
        this.displayName = displayName;
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
