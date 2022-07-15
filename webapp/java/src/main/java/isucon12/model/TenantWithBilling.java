package isucon12.model;

public class TenantWithBilling {
    private String id;
    private String name;
    private String displayName;
    private Long billingYen;

    public String getId() {
        return id;
    }
    public void setId(String id) {
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
    public Long getBillingYen() {
        return billingYen;
    }
    public void setBillingYen(Long billingYen) {
        this.billingYen = billingYen;
    }
}
