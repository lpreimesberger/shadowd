
# Shadowy

Shadowy is a completely postquantum blockchain using ml-dsa87 and built on Tendermint for consensus.  It features single binary deploys, so the single fat binary contains:

- Current Chain Genesis
- Hard coded seed pool

The total SHADOW pool is 21 million, with a 20 year mine out period.  There is no premine.  Mining is based on finding 'best' solutions for the previous
block puzzle from a pool of 'plots' on the local system and signing with it

Because OpenSSL 3.5.x has many performance issues - this uses CloudFlare's Circle crypto library for all operations.

# Options

--seeds - provides a list of seeds to communicate with for the mempool
--quiet - disables most Tendermint chatter
