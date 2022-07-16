package isucon12.json;

import com.fasterxml.jackson.annotation.JsonProperty;

public class FailureResult {
    @JsonProperty("status")
    private Boolean success;
    private String message;

    public FailureResult(Boolean success, String message) {
        super();
        this.success = success;
        this.message = message;
    }

    public Boolean getSuccess() {
        return success;
    }

    public void setSuccess(Boolean success) {
        this.success = success;
    }

    public String getMessage() {
        return message;
    }

    public void setMessage(String message) {
        this.message = message;
    }
}
