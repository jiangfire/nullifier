import { describe, expect, it } from 'vitest';
import { encodeScalarHex } from './field';
import { mimcConstants, mimcHash, mimcRoundCount } from './mimc';

describe('mimcConstants', () => {
  it('轮数与 gnark 一致', () => {
    expect(mimcConstants()).toHaveLength(mimcRoundCount);
    expect(mimcRoundCount).toBe(91);
  });

  it('首尾轮常量与 Go 侧一致', () => {
    const constants = mimcConstants();
    expect(encodeScalarHex(constants[0])).toBe(
      '00808370c37267481fb91b077899955706f209e5e0762dac2c79ba1e7a91b018',
    );
    expect(encodeScalarHex(constants[constants.length - 1])).toBe(
      '0ad024352c40ea93df089e7950a8ef31949ca31185ea5554e507f1f5b71a82a8',
    );
  });

  it('每次返回同一份常量', () => {
    expect(mimcConstants()).toBe(mimcConstants());
  });
});

describe('mimcHash', () => {
  it('与 Go 侧的 nullifier 派生逐位一致', () => {
    // Go: ballot.DeriveNullifier(1, DeriveProposalField("proposal-1"))
    const proposalField = BigInt(
      '0x22e971ef187286f3238ccf7f6552a1605434b5fc3684ef3b642cf011166b253f',
    );
    expect(encodeScalarHex(mimcHash(1n, proposalField))).toBe(
      '233e35c4c857f6cb21a2942fcd0d69212e5fd917b38270a3798ae6a70f97065c',
    );
    expect(encodeScalarHex(mimcHash(2n, proposalField))).toBe(
      '15777d64eefe8d60ab7b54adef75f9b26ce9630546484a839553dfb5d5415021',
    );
  });

  it('参数顺序不同会得到不同的结果', () => {
    const a = 7n;
    const b = 11n;
    expect(mimcHash(a, b)).not.toBe(mimcHash(b, a));
  });

  it('结果落在域内', () => {
    const modulus =
      21888242871839275222246405745257275088548364400416034343698204186575808495617n;
    expect(mimcHash(1n, 2n)).toBeLessThan(modulus);
  });
});
