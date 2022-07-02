import flask

app = flask.Flask(__name__)

if __name__ == "__main__":
    app.run(port=3000, debug=True, threaded=True)
