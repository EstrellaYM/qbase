package txs

import (
	"fmt"

	"github.com/QOSGroup/qbase/context"
	"github.com/QOSGroup/qbase/types"
	"github.com/pkg/errors"
	"github.com/tendermint/tendermint/crypto"
)

// 功能：抽象具体的Tx结构体
type ITx interface {
	ValidateData(ctx context.Context) error //检测

	//执行业务逻辑,
	// crossTxQcp: 需要进行跨链处理的TxQcp。
	// 业务端实现中crossTxQcp只需包含`to` 和 `txStd`
	Exec(ctx context.Context) (result types.Result, crossTxQcp *TxQcp)
	GetSigner() []types.Address //签名者
	CalcGas() types.BigInt      //计算gas
	GetGasPayer() types.Address //gas付费人
	GetSignData() []byte        //获取签名字段
}

// 标准Tx结构体
type TxStd struct {
	ITx       ITx          `json:"itx"`      //ITx接口，将被具体Tx结构实例化
	Signature []Signature  `json:"sigature"` //签名数组
	ChainID   string       `json:"chainid"`  //ChainID: 执行ITx.exec方法的链ID
	MaxGas    types.BigInt `json:"maxgas"`   //Gas消耗的最大值
}

var _ types.Tx = (*TxStd)(nil)

// 签名结构体
type Signature struct {
	Pubkey    crypto.PubKey `json:"pubkey"`    //可选
	Signature []byte        `json:"signature"` //签名内容
	Nonce     int64         `json:"nonce"`     //nonce的值
}

// Type: just for implements types.Tx
func (tx *TxStd) Type() string {
	return "txstd"
}

// 将需要签名的字段拼接成 []byte
func (tx *TxStd) GetSignData() []byte {
	if tx.ITx == nil {
		panic("ITx shouldn't be nil in TxStd.GetSignData()")
		return nil
	}

	ret := tx.ITx.GetSignData()
	ret = append(ret, []byte(tx.ChainID)...)
	ret = append(ret, types.Int2Byte(tx.MaxGas.Int64())...)

	return ret
}

// 签名：每个签名者外部调用此方法
func (tx *TxStd) SignTx(privkey crypto.PrivKey, nonce int64) (signedbyte []byte, err error) {
	if tx.ITx == nil {
		return nil, errors.New("Signature txstd err(itx is nil)")
	}

	sigdata := append(tx.GetSignData(), types.Int2Byte(nonce)...)
	signedbyte, err = privkey.Sign(sigdata)
	if err != nil {
		return nil, err
	}

	return
}

// 构建结构体
// 调用 NewTxStd后，需调用TxStd.SignTx填充TxStd.Signature(每个TxStd.Signer())
func NewTxStd(itx ITx, cid string, mgas types.BigInt) (rTx *TxStd) {
	rTx = &TxStd{
		itx,
		[]Signature{},
		cid,
		mgas,
	}

	return
}

// 函数：Signature结构转化为 []byte
func Sig2Byte(sgn Signature) (ret []byte) {
	if sgn.Pubkey == nil {
		return nil
	}
	ret = append(ret, sgn.Pubkey.Bytes()...)
	ret = append(ret, sgn.Signature...)
	ret = append(ret, types.Int2Byte(sgn.Nonce)...)

	return
}

//ValidateBasicData  对txStd进行基础的数据校验
//tx.ITx == QcpTxResult时 不校验签名相关信息
func (tx *TxStd) ValidateBasicData(ctx context.Context, isCheckTx bool, currentChaindID string) (err types.Error) {
	if tx.ITx == nil {
		return types.ErrInternal("TxStd's ITx is nil")
	}

	//开启cache执行ITx.ValidateData，在ITx.ValidateData中做数据保存操作将被忽略
	newCtx, _ := ctx.CacheContext()
	itxErr := tx.ITx.ValidateData(newCtx)
	if itxErr != nil {
		return types.ErrInternal(fmt.Sprintf("TxStd's ITx ValidateData error:  %s", itxErr.Error()))
	}

	if tx.ChainID == "" {
		return types.ErrInternal("TxStd's ChainID is empty")
	}

	if tx.ChainID != currentChaindID {
		return types.ErrInternal(fmt.Sprintf("chainId not match. expect: %s , actual: %s", currentChaindID, tx.ChainID))
	}

	if tx.MaxGas.LT(types.ZeroInt()) {
		return types.ErrInternal("TxStd's MaxGas is less than zero")
	}

	execGas := tx.ITx.CalcGas()
	if tx.MaxGas.LT(execGas) {
		return types.ErrInternal(fmt.Sprintf("TxStd's MaxGas is less than itx exec gas. expect: %s , actual: %s", tx.MaxGas, execGas))
	}

	_, ok := tx.ITx.(*QcpTxResult)
	if !ok {

		singers := tx.ITx.GetSigner()
		if len(singers) == 0 {
			return
		}

		sigs := tx.Signature
		if len(sigs) == 0 {
			return types.ErrUnauthorized("no signatures in TxStd's ITx")
		}

		if len(sigs) != len(singers) {
			return types.ErrUnauthorized(fmt.Sprintf("signatures and signers not match. signatures count: %d , singers count: %d ", len(sigs), len(singers)))
		}
	}

	return
}
