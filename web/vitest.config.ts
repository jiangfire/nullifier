import { defineConfig } from 'vitest/config';
import { compileScript, parse } from 'vue/compiler-sfc';
import ts from 'typescript';

/**
 * 给每个 `.vue` 文件算一个稳定的短 id（scoped CSS 的 scopeId 用）。
 *
 * 用 FNV-1a 而不是 `node:crypto` 的哈希：这里要的只是「同一文件每次都一样、
 * 不同文件不一样」，不需要抗碰撞的密码学强度 —— 而 `node:crypto` 会让这个
 * 配置文件依赖 node 的类型声明（本仓库没装 `@types/node`）。
 */
function scopeIdOf(filename: string): string {
  let hash = 0x811c9dc5;
  for (let index = 0; index < filename.length; index += 1) {
    hash ^= filename.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash.toString(16).padStart(8, '0');
}

/**
 * 把 `.vue` 单文件组件编译成可执行的 ES 模块。
 *
 * **为什么自己写**：离线环境装不了 `@vitejs/plugin-vue`（模块缓存里没有），
 * 而 `vue` 这个包本身就带 `compiler-sfc`（`vue/compiler-sfc` 是它的再导出），
 * `typescript` 又已经是 devDependencies 里的依赖 —— 三样凑齐就不需要新装包。
 *
 * **覆盖范围**：本仓库的 17 个 `.vue` 全部是 `<script setup>`，因此走
 * `compileScript({ inlineTemplate: true })` 这一条路：模板被编译进 `setup` 的
 * 返回值里，产物是单个模块，不需要再拼 `render` 选项。
 *
 * **不覆盖**：`<style>` 块（测试里不需要样式）、`src=` 外部引用。
 */
function vueSfcPlugin() {
  return {
    name: 'nullifier:vue-sfc',
    enforce: 'pre' as const,
    async transform(code: string, id: string) {
      const filename = id.split('?')[0];
      if (!filename.endsWith('.vue')) {
        return null;
      }

      const { descriptor, errors } = parse(code, { filename });
      if (errors.length > 0) {
        throw new Error(`SFC 解析失败（${filename}）：${errors[0].message}`);
      }
      if (!descriptor.scriptSetup) {
        // 没有 script setup 就没法走 inlineTemplate。与其渲染出一个空组件，
        // 不如直接报错 —— 静默的空渲染比失败更难查。
        throw new Error(
          `SFC 编译失败（${filename}）：只支持 <script setup>，本仓库的组件都是这个形态`,
        );
      }

      const scopeId = scopeIdOf(filename);
      // compileScript 的产物自带合法的 `export default`（`genDefaultAs` 与
      // `rewriteDefault` 都不需要 —— 后者只对函数声明形式的默认导出生效，
      // 而这里导出的是 `_defineComponent({...})` 这个调用表达式）。
      const script = compileScript(descriptor, {
        id: scopeId,
        inlineTemplate: true,
        babelParserPlugins: ['typescript'],
        templateOptions: {
          compilerOptions: { scopeId: `data-v-${scopeId}` },
        },
      });

      // 编译产物里仍有 TypeScript 语法（`import type`、模板事件处理器的
      // `($event: any)`），要再走一遍 TS → JS。
      const transpiled = ts.transpileModule(script.content, {
        // transpileModule 按扩展名判断脚本类型 —— `.vue` 是它不认识的扩展名，
        // 于是按 JS 解析，`import type { X } from '...'` 直接语法错误。
        // 给一个 `.ts` 后缀，它就会按 TypeScript 解析。
        fileName: `${filename}.ts`,
        compilerOptions: {
          target: ts.ScriptTarget.ES2020,
          module: ts.ModuleKind.ESNext,
        },
      });
      return { code: transpiled.outputText, map: null };
    },
  };
}

export default defineConfig({
  plugins: [vueSfcPlugin()],
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
    setupFiles: ['./test/setup.ts'],
  },
});
