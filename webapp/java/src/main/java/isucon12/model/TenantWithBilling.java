package isucon12.model;

import com.fasterxml.jackson.annotation.JsonProperty;

public class TenantWithBilling {
    private String id;
    private String name;
    @JsonProperty("display_name")
    private String displayName;
    @JsonProperty("billing")
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
