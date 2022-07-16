package isucon12.model;

public class Viewer {
    private String role;
    private String playerId;
    private String tenantName;
    private Long tenantId;

    public Viewer(String role, String playerId, String tenantName, Long tenantId) {
        super();
        this.role = role;
        this.playerId = playerId;
        this.tenantName = tenantName;
        this.tenantId = tenantId;
    }

    public String getRole() {
        return role;
    }

    public void setRole(String role) {
        this.role = role;
    }

    public String getPlayerId() {
        return playerId;
    }

    public void setPlayerId(String playerId) {
        this.playerId = playerId;
    }

    public String getTenantName() {
        return tenantName;
    }

    public void setTenantName(String tenantName) {
        this.tenantName = tenantName;
    }

    public Long getTenantId() {
        return tenantId;
    }

    public void setTenantId(Long tenantId) {
        this.tenantId = tenantId;
    }

    @Override
    public String toString() {
        return "Viewer [role=" + role + ", playerId=" + playerId + ", tenantName=" + tenantName + ", tenantId=" + tenantId + "]";
    }
}
