package isucon12.json;

import java.util.List;

public class PlayerHandlerResult {
    private PlayerDetail player;
    private List<PlayerScoreDetail> scores;

    public PlayerHandlerResult(PlayerDetail player, List<PlayerScoreDetail> scores) {
        super();
        this.player = player;
        this.scores = scores;
    }

    public PlayerDetail getPlayer() {
        return player;
    }

    public void setPlayer(PlayerDetail player) {
        this.player = player;
    }

    public List<PlayerScoreDetail> getScores() {
        return scores;
    }

    public void setScores(List<PlayerScoreDetail> scores) {
        this.scores = scores;
    }
}
