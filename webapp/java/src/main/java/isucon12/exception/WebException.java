package isucon12.exception;

import org.springframework.http.HttpStatus;

public class WebException extends RuntimeException {
    private static final long serialVersionUID = -8335566825747001661L;
    private final HttpStatus httpStatus;
    private final String errorMessage;

    public WebException(HttpStatus httpStatus, Throwable cause) {
        this(httpStatus, null, cause);
    }

    public WebException(HttpStatus httpStatus, String errorMessage) {
        this(httpStatus, errorMessage, null);
    }

    public WebException(HttpStatus httpStatus, String errorMessage, Throwable cause) {
        super(cause);
        this.httpStatus = httpStatus;
        this.errorMessage = errorMessage;
    }

    public HttpStatus getHttpStatus() {
        return httpStatus;
    }

    public String getErrorMessage() {
        return errorMessage;
    }
}
