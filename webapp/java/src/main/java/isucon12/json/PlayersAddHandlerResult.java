package isucon12.json;

import java.util.List;

public class PlayersAddHandlerResult {
    private List<PlayerDetail> players;

    public PlayersAddHandlerResult(List<PlayerDetail> players) {
        super();
        this.players = players;
    }

    public List<PlayerDetail> getPlayers() {
        return players;
    }

    public void setPlayers(List<PlayerDetail> players) {
        this.players = players;
    }
}
