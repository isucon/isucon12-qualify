package isucon12.json;

import com.fasterxml.jackson.annotation.JsonProperty;

public class MeHandlerResult {
    private TenantDetail tenant;
    private PlayerDetail me;
    private String role;
    @JsonProperty("logged_in")
    private Boolean loggedIn;

    public MeHandlerResult(TenantDetail tenant, PlayerDetail me, String role, Boolean loggedIn) {
        super();
        this.tenant = tenant;
        this.me = me;
        this.role = role;
        this.loggedIn = loggedIn;
    }

    public TenantDetail getTenant() {
        return tenant;
    }

    public void setTenant(TenantDetail tenant) {
        this.tenant = tenant;
    }

    public PlayerDetail getMe() {
        return me;
    }

    public void setMe(PlayerDetail me) {
        this.me = me;
    }

    public String getRole() {
        return role;
    }

    public void setRole(String role) {
        this.role = role;
    }

    public Boolean getLoggedIn() {
        return loggedIn;
    }

    public void setLoggedIn(Boolean loggedIn) {
        this.loggedIn = loggedIn;
    }
}
