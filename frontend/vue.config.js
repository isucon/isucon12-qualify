const { defineConfig } = require("@vue/cli-service");
module.exports = defineConfig({
  transpileDependencies: true,
  productionSourceMap: false,
  outputDir: '../public',
  devServer: {
    allowedHosts: ['.t.isucon.dev', '.powawa.net'],
  },
});
