package isucon12.json;

import java.util.List;

public class CompetitionRankingHandlerResult {
    private CompetitionDetail competition;
    private List<CompetitionRank> ranks;

    public CompetitionRankingHandlerResult(CompetitionDetail competition, List<CompetitionRank> ranks) {
        super();
        this.competition = competition;
        this.ranks = ranks;
    }

    public CompetitionDetail getCompetition() {
        return competition;
    }

    public void setCompetition(CompetitionDetail competition) {
        this.competition = competition;
    }

    public List<CompetitionRank> getRanks() {
        return ranks;
    }

    public void setRanks(List<CompetitionRank> ranks) {
        this.ranks = ranks;
    }
}
