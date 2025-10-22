# On-chain Swaps

SHADOW swaps are implemented as a special NFT/token created as a TX_OFFER.

Parts of this offer are:
- have (a token id of an active token on chain)
- want (a token id of an active token on chain)
- have_amount (an amount of the have token)
- want_amount (an amount of the want token)
- good_until - max number of blocks this offer is active in mempool, defaults to cache max (around 2 weeks)
- regular fees - paid in SHADOW for the offer to be distributed

Much like minting a new token, a TX_OFFER locks the have-token amount, which become unavailable.  If not accepted,
the UTXO inputs that are locked return to the offer creator on expiry.  

A TX_OFFER is accepted by a matching TX_ACCEPT, which has the tx_id of the offer as part of the transaction.  The TX_ACCEPT must
contain want_amount of the 'want' token along with appropriate SHADOW fees, and will result in the input being assigned to the 
account originally submitting the TX_OFFER, and the locked offer tokens are released to the TX_ACCEPT account.

Offers are not reversible.  The original offer submitter can TX_MELT the offer to cancel it, but once accepted there is no rollback.


1) Option b, we don't need tokens running all over, good catch!
2) yeah - let's put in the block, i was thinking mempool is simpler to implement, but we have no tracking then.  that means that the fees for an offer are gone, which is good.  less noise!
3) no partial fills - all or nothing for these.  if they want just some they need to go to a pool
4) originally was just going to use TX_MELT, but you're right, TX_CANCEL and that will match the chain entries

refinements look good and added to doc - anything else you can think of before we start?
