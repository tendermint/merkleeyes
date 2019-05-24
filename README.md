# merkleeyes

[![CircleCI](https://circleci.com/gh/tendermint/merkleeyes.svg?style=svg)](https://circleci.com/gh/tendermint/merkleeyes)

A simple [ABCI application](http://github.com/tendermint/abci) serving a [merkle-tree key-value store](http://github.com/tendermint/merkleeyes/iavl) 

This once served as the application used for the [Jepsen testing of
Tendermint](https://jepsen.io/analyses/tendermint-0-10-2). It is long since
deprecated.

We hope to revive it using the
[Cosmos-SDK](https://github.com/cosmos/cosmos-sdk).

# Spec

The state consists of a key-value store and a validator set. 
The validator set contains a public key and voting power for each validator,
and a version number that increments once in every block where the validator set changes.

There are no accounts. Each transaction must contain a unique nonce.

It should have the following Msg types:

```
// Set Key=Value in the store
type MsgSet struct {
    Key []byte
    Value []byte
}

// Get the value stored under Key
type MsgGet struct {
    Key []byte
}

// Remove the Key and its value from the store
type MsgRemove struct {
    Key []byte
}


// If the value stored under Key is CompareValue, set it to SetValue
type MsgCompareAndSet struct {
    Key []byte
    CompareValue []byte
    SetValue []byte
}

// Add the Pubkey as a validator with the given Power.
// If the Pubkey is already a validator, set the voting power to Power.
// If Power=0, remove the validator from the set.
type MsgValidatorSetChange struct {
    Pubkey crypto.PubKey
    Power int64
}

// MsgReturn the validator set
type ValidatorSetRead struct {
}

// If Version matches the current validator set version, update the voting power for
the given validator Pubkey.
type MsgValidatorSetCompareAndSet struct {
    Version int64
    Pubkey crypto.Pubkey
    Power int64
}
```

# Use

WARNING: This code and instructions are deprecated. It refers to an application
implemented directly over the Tendermint ABCI. Applications should use the
[Cosmos-SDK](https://github.com/cosmos/cosmos-sdk) instead.

Merkleeyes allows inserts and removes by key, and queries by key or index.
Inserts and removes happen through the `DeliverTx` message, while queries happen through the `Query` message.
`CheckTx` simply mirrors `DeliverTx`.

# Formatting

## Byte arrays

Byte-array `B` is serialized to `Encode(B)` as follows:

```
Len(B) := Big-Endian encoded length of B
Encode(B) = Len(Len(B)) | Len(B) | B
```

So if `B = "eric"`, then `Encode(B) = 0x010465726963`

## Transactions

There are four types of transaction, each associated with a type-byte and a list of arguments:

```
Set			0x01		Key, Value
Remove			0x02		Key
Get			0x03		Key
Compare and Set		0x04		Key, Compare Value, Set Value
Validator Set Change    0x05		PubKey, Power (uint64)
Validator Set Read      0x06		
Validator Set CAS       0x07		Version (uint64), PubKey, Power (uint64)	
```

A transaction consists of a 12-byte random nonce, the type-byte, and the encoded arguments.

For instance, to insert a key-value pair, you would submit a transaction that looked like `NONCE | 01 | Encode(key) | Encode(value)`,
where `|` denotes concatenation.
Thus, a transaction inserting the key-value pair `(eric, clapton)` would look like:

```
0xF4FCDC5BF26E227B66A1BA90010104657269630107636c6170746f6e
```

The first 12-bytes, `F4FCDC5BF26E227B66A1BA90`, are the nonce. The next byte, `01`, is the transaction type.
Following that are the encodings of `eric` and `clapton`.


Here's a session from the [abci-cli](https://tendermint.com/intro/getting-started/first-abci):

```
# SET ("eric", "clapton")
> deliver_tx 0xF4FCDC5BF26E227B66A1BA90010104657269630107636c6170746f6e

# GET ("eric")
> deliver_tx 0xB980403FF73E79A3A2D90A1E03010465726963
-> data: clapton
-> data.hex: 636C6170746F6E

# CAS ("eric", "clapton", "ericson")
> deliver_tx 0x18D892B6D62773E6AA8804CF040104657269630107636C6170746F6E010765726963736f6e

# GET ("eric")
> deliver_tx 0x4FB9DAB513493E602FF085C603010465726963
-> data: ericson
-> data.hex: 65726963736F6E

# COMMIT
> commit
-> data: ���Ώ�R�Ng�=HK}��7�
-> data.hex: BAEDE5CE8F9A52B64E67873D484B7DABF69537DA

# QUERY ("eric")
> query 0x65726963
-> height: 2
-> key: eric
-> key.hex: 65726963
-> value: ericson
-> value.hex: 65726963736F6E
```


# Poem

To the tune of Eric Clapton's "My Father's Eyes"

```
writing down, my checksum
waiting for the, data to come
no need to pray for integrity
thats cuz I use, a merkle tree

grab the root, with a quick hash run
if the hash works out,
it must have been done

theres no need, for trust to arise
thanks to the crypto
now that I can merkleyes

take that data, merklize
ye, I merklize ...

then the truth, begins to shine
the inverse of a hash, you will never find
and as I watch, the dataset grow
producing a proof, is never slow

Where do I find, the will to hash
How do I teach it?
It doesn't pay in cash
Bitcoin, here, I've realized
Thats what I need now,
cuz real currencies merklize
-EB
```
