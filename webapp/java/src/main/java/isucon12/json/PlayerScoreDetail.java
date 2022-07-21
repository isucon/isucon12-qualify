package isucon12.json;

import com.fasterxml.jackson.annotation.JsonProperty;

public class PlayerScoreDetail {
    @JsonProperty("competition_title")
    private String competitionTitle;
    private Long score;

    public PlayerScoreDetail(String competitionTitle, Long score) {
        super();
        this.competitionTitle = competitionTitle;
        this.score = score;
    }

    public String getCompetitionTitle() {
        return competitionTitle;
    }

    public void setCompetitionTitle(String competitionTitle) {
        this.competitionTitle = competitionTitle;
    }

    public Long getScore() {
        return score;
    }

    public void setScore(Long score) {
        this.score = score;
    }
}
