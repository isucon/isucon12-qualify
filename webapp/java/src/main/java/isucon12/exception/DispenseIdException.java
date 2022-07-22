package isucon12.exception;

public class DispenseIdException extends Exception {
    private static final long serialVersionUID = 7362898518614679325L;

    public DispenseIdException(String message) {
        this(message, null);
    }

    public DispenseIdException(String message, Throwable cause) {
        super(message, cause);
    }
}
