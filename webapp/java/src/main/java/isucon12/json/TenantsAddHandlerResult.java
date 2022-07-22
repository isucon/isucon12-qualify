package isucon12.json;

import isucon12.model.TenantWithBilling;

public class TenantsAddHandlerResult {
    private TenantWithBilling tenant;

    public TenantsAddHandlerResult(TenantWithBilling tenant) {
        super();
        this.tenant = tenant;
    }

    public TenantWithBilling getTenant() {
        return tenant;
    }

    public void setTenant(TenantWithBilling tenant) {
        this.tenant = tenant;
    }
}
