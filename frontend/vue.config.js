const { defineConfig } = require("@vue/cli-service");
module.exports = defineConfig({
  transpileDependencies: true,
  devServer: {
    allowedHosts: ['.t.isucon.dev', '.powawa.net'],
    https: {
      key: '../nginx/tls/key.pem',
      cert: '../nginx/tls/fullchain.pem',
    },
  },
});
