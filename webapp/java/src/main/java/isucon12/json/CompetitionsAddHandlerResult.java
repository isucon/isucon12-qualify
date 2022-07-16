package isucon12.json;

public class CompetitionsAddHandlerResult {
    private CompetitionDetail competition;

    public CompetitionsAddHandlerResult(CompetitionDetail competition) {
        super();
        this.competition = competition;
    }

    public CompetitionDetail getCompetition() {
        return competition;
    }

    public void setCompetition(CompetitionDetail competition) {
        this.competition = competition;
    }
}
