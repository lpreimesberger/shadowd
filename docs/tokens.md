# Tokens


## SHADOW Token Specification
- Token ID: `0x0000000000000000000000000000000000000000000000000000000000000000` (reserved)
- Decimals: 8 (100,000,000 satoshis = 1 SHADOW)
- Max Supply: Set in genesis block (e.g., 21 million SHADOW + ongoing block rewards)
- Cannot be melted
- Minted only through coinbase transactions (block rewards)


SHADOW has 8 decimal places (100,000,000 satoshis = 1 SHADOW)

## Token ID Generation
- Token ID = Transaction ID of the TX_MINT transaction
- Guaranteed unique (TX IDs are unique)
- SHADOW uses reserved ID 0x00...00


The base token ('gas token' in Ethereum parlance) - is SHADOW, which is assigned a token id during chain creation.
All chain operations require that SHADOW be included as a network fee, although the amount is user set.  If zero,
it is unlikely the operation will be accepted prior to node timeout (configuration dependent).

All other tokens are created with a TX_MINT transaction type - which in addition to the standard tx inputs uses
the generic tx fields to include information on the token:

- TICKER (from 3 up to 32 ASCII chars, intended to be the short name of the token - required to be unique.  any token with zero remaining unmelted satoshi can have its ticker reused.)
- DESC (from zero up to 64 ASCII chars, space for project info or a url describing)
- MAX_MINT - how many base tokens will be created (cannot be changed later - max is 21 million base units)
- MAX_DECIMALS - how many 'satoshi' per base token
- MINT_VERSION - set to zero, this is for any future chain changes

Any fees for the minting operation are extra, beyond the staking to enforce the 1-1 SHADOW link.

Once minted, a token (or any on-chain entity) can be sent with TX_SEND with the token hash ID.  SHADOW is just a special case
of a token (the only perpetual token on chain)

A unique part of this chain is all non-SHADOW objects can be 'melted' and destroyed by the owner.  Token creators cannot
rug pull owners or spam random addresses with junk tokens.  Once a token is sent - even the token creator loses control.
Creating spam tokens is expensive and essentially giving money away, since end users can recycle them for SHADOW.  
SHADOW however connect be 'melted', sending a TX_MELT on SHADOW will fail.  To make
this possible - a TX_MINT operation requires an equal amount of SHADOW satoshi be staked.  This means:

- a token with 8 decimal places requires an equal amount of SHADOW be included in the send fields of the TX, which are tied to the tokens then
- this make it impossible to have a token the same 'size' as SHADOW since it would take all SHADOW in existence.  Users must reduce supply to match staking - worst case setting zero decimal places, when each base token is a single SHADOW satoshi
- zero decimal places is allowed
- TX_MELT returns proportional SHADOW to the original stake (the UTXO of the staking is released to the melter)


A Token can be referred to onchain by token_id = tx_id_of_mint_transaction/

Paired with TX_MINT is TX_MELT, which takes as input X custom tokens - removes them from being 'active' on chain and unlocks the base token again for the token owner.

Again for a worst case, if a wallet gets a custom token with 8 decimal places from another user - they can TX_MELT that token one for one with SHADOW, less fees, and those custom token utxo's are 'burned' and gone forever.

The incoming UTXOs for the mint become locked and can only be unlocked through TX_MELT.  These SHADOW UTXOs are sent proportionally to the token and can be returned to current owner through TX_MELT.

A custom TOKEN cannot have more decimal places than SHADOW since the UTXO are locked.  Staking is enforced by the chain - trying to mint more tokens than staked will fail.

The unique token ID used by the network will be the hash of the original TX minting it.  The TICKER name is unique on the network and enforced by the nodes at minting time.  Once all tokens are melted, it can be reused.


Total SHADOW is (or should be) locked to 21 million in genesis.
I think SHADOW has a TOKEN_ID set from the genesis now at least - it's not 0x0...0
ASCII fields are just A-Z, a-z, and 0-9 yeah - otherwise people can be tricky and use 'close' unicode chars to typosquat
MAX_MINT has a max BASE UNITS of 21 million.  token_id = tx_id_of_mint_transaction was added!
any token with zero remaining unmelted satoshi can have its ticker reused - was thinking track if not active and remove from the token list.
Any fees for the minting operation are extra, beyond the staking to enforce the 1-1 SHADOW link.
TX_MINT just makes a single UTXO, which is sent to the minter.
Partial melts are allowed (the end user won't understand which UTXO is, so up to use to make it usable)
