package isucon12;

import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.ControllerAdvice;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.ResponseBody;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.servlet.mvc.method.annotation.ResponseEntityExceptionHandler;

import isucon12.exception.WebException;
import isucon12.json.FailureResult;

/**
 * エラー処理
 */
@ControllerAdvice(annotations = { RestController.class })
public class RestControllerAdvice extends ResponseEntityExceptionHandler {
    Logger logger = LoggerFactory.getLogger(RestControllerAdvice.class);

    @ExceptionHandler(WebException.class)
    @ResponseBody
    public FailureResult handlerWebException(HttpServletRequest req, HttpServletResponse res, WebException e) {
        logger.error("error at {}: status={}: message={}, errorMessage={}", req.getRequestURI(), e.getHttpStatus().value(), e.getMessage(), e.getErrorMessage());
        res.setStatus(e.getHttpStatus().value());
        return new FailureResult(false, e.getErrorMessage());
    }

    @ExceptionHandler(Throwable.class)
    @ResponseStatus(value = HttpStatus.INTERNAL_SERVER_ERROR)
    @ResponseBody
    public FailureResult handlerException(HttpServletRequest req, HttpServletResponse res, Throwable t) {
        logger.error("error at {}: {}", req.getRequestURI(), t.getMessage(), t);
        return new FailureResult(false, t.getMessage());
    }

}
