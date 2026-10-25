import {
  defineConfigWithVueTs,
  vueTsConfigs,
} from '@vue/eslint-config-typescript';
import { globalIgnores } from 'eslint/config';
import pluginVue from 'eslint-plugin-vue';
import globals from 'globals';

// To allow more languages other than `ts` in `.vue` files, uncomment the following lines:
// import { configureVueProject } from '@vue/eslint-config-typescript'
// configureVueProject({ scriptLangs: ['ts', 'tsx'] })
// More info at https://github.com/vuejs/eslint-config-typescript/#advanced-setup

export default defineConfigWithVueTs(
  {
    name: 'app/files-to-lint',
    files: ['**/*.{ts,mts,tsx,vue}'],
  },
  // `public/` 放的是**构建产物**：Go 官方胶水 `wasm_exec.js`、浏览器端 prover
  // 的 `ballot-prover.wasm`、以及只在客户端使用的 `proving.key`。
  // 它们不是本仓库的源码（胶水来自 GOROOT，wasm 由 `make web-wasm` 生成），
  // 不该被 lint —— 顺带也避免 proving.key 之类的东西被工具链误读。
  globalIgnores([
    '**/dist/**',
    '**/dist-ssr/**',
    '**/coverage/**',
    '**/public/**',
  ]),
  { languageOptions: { globals: globals.browser } },
  pluginVue.configs['flat/essential'],
  vueTsConfigs.recommended,
);
