package isucon12.json;

import java.util.List;

public class CompetitionsHandlerResult {
    private List<CompetitionDetail> competitions;

    public CompetitionsHandlerResult(List<CompetitionDetail> competitions) {
        super();
        this.competitions = competitions;
    }

    public List<CompetitionDetail> getCompetitions() {
        return competitions;
    }

    public void setCompetitions(List<CompetitionDetail> competitions) {
        this.competitions = competitions;
    }
}
