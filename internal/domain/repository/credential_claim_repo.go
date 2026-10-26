package repository

import (
	"context"
	"time"

	"anonvote/internal/domain/entity"
)

// CredentialClaimRepository 读写凭证池。
type CredentialClaimRepository interface {
	// Save 批量写入待领取的槽位，返回实际写入的数量。
	Save(ctx context.Context, claims []*entity.CredentialClaim) error
	// ReserveAvailable **原子地**预留一个空闲槽位。
	//
	// 「原子」是硬要求：并发领取时，所有人都会看到同一个「第一个空闲槽位」，
	// 然后互相撞车（抢到的那个槽位被别人先领走）。预留把槽位占住，让并发
	// 的人各拿各的。
	//
	// 预留**会过期**：ttl 之前的预留视为无效，槽位重新可用 —— 只预留不领取
	// 的客户端不能永久占住池子。
	ReserveAvailable(
		ctx context.Context,
		proposalID string,
		ttl time.Duration,
	) (*entity.CredentialClaim, error)
	// ReleaseReservation 释放一次预留（领取流程后续失败时调用）。
	//
	// 槽位回到「空闲」状态 —— 凭证没签发，槽位不该被白白消耗。
	ReleaseReservation(ctx context.Context, claimID string) error
	// GetAvailableByUserID 按 user_id 查一个尚未领取的槽位。
	//
	// 真两段式盲签名要靠它把「盲签名」绑定到具体槽位：公开端点只能为池里
	// 还空着的槽位盲签，否则签发方就成了任意消息的签名预言机。
	GetAvailableByUserID(
		ctx context.Context,
		proposalID string,
		userID string,
	) (*entity.CredentialClaim, error)
	// GetReservedByUserID 按 user_id 查一个**正在预留中**（且未过期）的槽位。
	//
	// 公开的盲签名端点用它把签名绑定到「有人正在领的」槽位：预留过期的槽位
	// 可能已经被别人重新预留，签它等于替别人签名。
	GetReservedByUserID(
		ctx context.Context,
		proposalID string,
		userID string,
		ttl time.Duration,
	) (*entity.CredentialClaim, error)
	// MarkClaimed 原子地把槽位标记为已领取并写入领取者的公钥与签名。
	// 槽位已被别人领走时返回 false，调用方应重试。
	MarkClaimed(
		ctx context.Context,
		claimID string,
		userPubKey []byte,
		signature []byte,
		claimedAt time.Time,
	) (bool, error)
	// CountAvailable 返回该提案还剩多少未领取的槽位。
	CountAvailable(ctx context.Context, proposalID string) (int64, error)
}
