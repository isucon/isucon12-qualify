package isucon12.json;

import com.fasterxml.jackson.annotation.JsonProperty;

public class TenantDetail {
    private String name;
    @JsonProperty("display_name")
    private String displayName;

    public TenantDetail(String name, String displayName) {
        super();
        this.name = name;
        this.displayName = displayName;
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
}
