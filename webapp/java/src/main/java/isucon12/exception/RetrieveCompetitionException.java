package isucon12.exception;

public class RetrieveCompetitionException extends Exception {
    private static final long serialVersionUID = 234392988961105682L;

    public RetrieveCompetitionException(String message) {
        this(message, null);
    }

    public RetrieveCompetitionException(String message, Throwable cause) {
        super(message, cause);
    }
}
