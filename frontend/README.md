# isuports

## Project setup
```
npm install
```

### Compiles and hot-reloads for development
```
npm run serve
```

### Compiles and minifies for production
```
npm run build
```

### Lints and fixes files
```
npm run lint
```

### お手元で開発する

- `./frontend/docker-compose.override.yml` を読むようにする
  - プロジェクトルートで `ln -s frontend/docker-compose.override.yml docker-compose.override.yml` する
- docker compose up --build で普通に起動する

あとは普通に編集したらwebpack-dev-serverがhotreloadしてくれます。


### Customize configuration
See [Configuration Reference](https://cli.vuejs.org/config/).
