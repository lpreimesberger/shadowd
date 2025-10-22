# Liquidity

Liquidity is provided through on-chain mechanics similar to swaps.  Users can 'swap' one supported token for the other
on a sliding exchange rate for a small fee (which goes to liquidity holders).

Any user with sufficient liquidity can create a liquidity pool through issuing a TX_LIQUIDITY_CREATE transaction on network.
This transaction takes as inputs:


- Token A - a hash for the first token
- Token A Locked - a starting lock value of A
- Token B - a hash for the first token
- Token B Locked - a starting lock value of B
- Fee ratio - must be between .1% and 10% for high risk tokens
 
 
Implicit in this is the K Value - the solution to amount(a) * amount(b), this is a constant set at create time.  The initial ratio
set by the creator will be maintained for swaps as long as the pool exists (i.e. - the exchange rate will float to maintain
the initial ratio set on creation).  Also implicit is the pool name, which defaults to "{Ticker A} / {Ticker B} - {K Ratio} - {TX Hash[8:]}".

Creating a pool moves the UTXO of all involved tokens to the pool and they are no longer under user control.

Creating a pool will generate a pool hash address (like used in tokens), and a token_id based on the minting hash.  Max supply of 
liquidity tokens is based on the largest of the max supplies in the swap (i.e. - if SHADOW is used, the max supply is 21 million).
The original creator of the pool receives tokens equal to the amount of satoshi locked in the original TX, so if a user creates a
new pool for the token CATS, which a max supply of 1000 paired with SHADOW, if they create with 10 CATS, 10 SHADOW:

    - the max supply of the new token is 21 million (SHADOW is more)
    - they will receive 10 top level units of the new token

Future investors can lock additional value through TX_LIQUIDITY_ADD, which takes

- the liquidity pool address 
- Token A - a hash for the first token
- Token A Locked - amount of token A to add
- Token B - a hash for the first token

The amount of token B is computed to keep the original K balance, and will be locked and removed from the user account.  The user will
receive tokens as above.

Investors can redeem by melting their liquidity tokens for the underlaying pair:

TX_LIQUIDITY_REMOVE takes:

- a liquidity token hash/token_id
- an amount of that token

It will destroy the amount offered, and release a proportional amount of A and B to the issuer.  Allowing them to cash out.

Regular users can use the pool through TX_LP_SWAP, which takes:

- a token_id (one of A or B)
- an amount

The user receives the matching token to their token_id adjusted for the locked ratio of the pool, minus the fee percentage of the input 
token (the fee is taken off the top of the input and added to the pool balances).










