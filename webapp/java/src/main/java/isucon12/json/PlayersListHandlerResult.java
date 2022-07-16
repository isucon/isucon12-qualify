package isucon12.json;

import java.util.List;

public class PlayersListHandlerResult {
    private List<PlayerDetail> players;

    public PlayersListHandlerResult(List<PlayerDetail> players) {
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
