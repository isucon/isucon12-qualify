package isucon12.json;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;

public class SuccessResult {
    @JsonProperty("status")
    private Boolean success;
    @JsonInclude(JsonInclude.Include.NON_EMPTY)
    private Object data;

    public SuccessResult(Boolean success, Object data) {
        this.success = success;
        this.data = data;
    }

    public Boolean getSuccess() {
        return success;
    }

    public void setSuccess(Boolean success) {
        this.success = success;
    }

    public Object getData() {
        return data;
    }

    public void setData(Object data) {
        this.data = data;
    }
}
