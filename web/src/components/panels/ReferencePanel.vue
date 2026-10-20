<script setup lang="ts">
const terms: Array<{ term: string; desc: string }> = [
  {
    term: 'proposal_id',
    desc: '一场投票的唯一编号，后续所有操作都靠它指向同一场投票。',
  },
  {
    term: 'user_id',
    desc: '用户标识，用于校验投票资格，不会与票内容绑成明文表。',
  },
  {
    term: 'nullifier',
    desc: '一次性防重号，防止同一张匿名票被重复提交。',
  },
  {
    term: 'proof',
    desc: '证明这张票合法的材料。当前版本还没有真实的选票电路，开发模式下由服务端代生成，联调时可自动填充。',
  },
  {
    term: 'encrypted_vote',
    desc: '加密后的投票内容，服务端会保存它。当前版本密文由服务端生成，生成时服务端能看到你选的选项；「服务端看不到选项」要等客户端本地加密落地。',
  },
  {
    term: 'threshold / trustees',
    desc: 'threshold 是至少需要几份，trustees 是总共拆成几份。2/3 表示三份里任意两份可用。',
  },
  {
    term: 'shares',
    desc: '解密份额，初始化解密时生成，发布结果时再提交回来。',
  },
  {
    term: 'anchor_hash / merkle_root',
    desc: '结果校验指纹，用于技术审计与留痕核验。',
  },
];

const userSteps: Array<{ title: string; text: string }> = [
  {
    title: '选提案',
    text: '在投票大厅选中要参与的提案，系统自动带上 proposal_id。',
  },
  { title: '领凭证', text: '在「我的身份」为当前提案领取匿名凭证。' },
  { title: '选选项', text: '在「参与投票」选择候选项。' },
  { title: '生成并提交', text: '按选项生成凭据，确认材料已生成后提交选票。' },
];

const adminSteps: Array<{ title: string; text: string }> = [
  { title: '创建提案', text: '先创建一场尚未开始的提案，拿到 proposal_id。' },
  { title: '初始化解密', text: '投票开始前生成 public_key 与 shares。' },
  {
    title: '签发凭证',
    text: '按提案为参与者签发凭证，A 提案的凭证不能投 B 提案。',
  },
  {
    title: '发布结果',
    text: '投票结束后提交足够的 shares，解密并发布最终结果。',
  },
];
</script>

<template>
  <div class="stack" style="--stack-gap: var(--s-5)">
    <div class="grid-2">
      <article class="card">
        <h2 class="card-title">投票人路径</h2>
        <ol class="ordered">
          <li v-for="item in userSteps" :key="item.title">
            <div>
              <p class="ordered-title">{{ item.title }}</p>
              <p class="ordered-text">{{ item.text }}</p>
            </div>
          </li>
        </ol>
      </article>

      <article class="card">
        <h2 class="card-title">运营路径</h2>
        <ol class="ordered">
          <li v-for="item in adminSteps" :key="item.title">
            <div>
              <p class="ordered-title">{{ item.title }}</p>
              <p class="ordered-text">{{ item.text }}</p>
            </div>
          </li>
        </ol>
      </article>
    </div>

    <article class="card">
      <h2 class="card-title">术语</h2>
      <dl class="defs">
        <div v-for="item in terms" :key="item.term" class="defs-item">
          <dt class="defs-term">{{ item.term }}</dt>
          <dd class="defs-desc">{{ item.desc }}</dd>
        </div>
      </dl>
    </article>
  </div>
</template>
