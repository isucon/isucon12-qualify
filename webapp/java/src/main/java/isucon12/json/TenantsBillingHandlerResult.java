package isucon12.json;

import java.util.List;

import isucon12.model.TenantWithBilling;

public class TenantsBillingHandlerResult {
    private List<TenantWithBilling> tenants;

    public List<TenantWithBilling> getTenants() {
        return tenants;
    }

    public void setTenants(List<TenantWithBilling> tenants) {
        this.tenants = tenants;
    }
}
