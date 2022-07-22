package isucon12.json;

public class PlayerDisqualifiedHandlerResult {
    private PlayerDetail player;

    public PlayerDisqualifiedHandlerResult(PlayerDetail player) {
        super();
        this.player = player;
    }

    public PlayerDetail getPlayer() {
        return player;
    }

    public void setPlayer(PlayerDetail player) {
        this.player = player;
    }
}
