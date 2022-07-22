package isucon12.json;

import com.fasterxml.jackson.annotation.JsonProperty;

public class CompetitionDetail {
    private String id;
    private String title;
    @JsonProperty("is_finished")
    private Boolean isFinished;

    public CompetitionDetail(String id, String title, Boolean isFinished) {
        super();
        this.id = id;
        this.title = title;
        this.isFinished = isFinished;
    }

    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    public String getTitle() {
        return title;
    }

    public void setTitle(String title) {
        this.title = title;
    }

    public Boolean getIsFinished() {
        return isFinished;
    }

    public void setIsFinished(Boolean isFinished) {
        this.isFinished = isFinished;
    }
}
