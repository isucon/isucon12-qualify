const { defineConfig } = require("@vue/cli-service");
module.exports = defineConfig({
  transpileDependencies: true,
  outputDir: '../public',
  devServer: {
    allowedHosts: ['.t.isucon.dev', '.powawa.net'],
    https: {
      key: '../nginx/tls/key.pem',
      cert: '../nginx/tls/fullchain.pem',
    },
    proxy: {
      '/api': {
        target: 'https://127.0.0.1:443',
        secure: false, // Do NOT check upstream cert's CN
      },
      '/auth': {
        target: 'https://127.0.0.1:443',
        secure: false, // Do NOT check upstream cert's CN
      },
    },
  },
});
