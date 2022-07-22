package isucon12.json;

import com.fasterxml.jackson.annotation.JsonProperty;

public class PlayerDetail {
    private String id;
    @JsonProperty("display_name")
    private String displayName;
    @JsonProperty("is_disqualified")
    private Boolean IsDisqualified;

    public PlayerDetail() {
        super();
    }

    public PlayerDetail(String id, String displayName, Boolean isDisqualified) {
        super();
        this.id = id;
        this.displayName = displayName;
        IsDisqualified = isDisqualified;
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
        return IsDisqualified;
    }

    public void setIsDisqualified(Boolean isDisqualified) {
        IsDisqualified = isDisqualified;
    }
}
