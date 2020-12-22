package schema

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-car"
	"github.com/stretchr/testify/require"
	cbg "github.com/whyrusleeping/cbor-gen"
	"io/ioutil"
	"reflect"
	"testing"
)

func TestRandomnessCircularSerde(t *testing.T) {
	tv1 := TestVector{
		Randomness: Randomness{
			{
				On: RandomnessRule{
					Kind:                RandomnessBeacon,
					DomainSeparationTag: 5,
					Epoch:               10,
					Entropy:             []byte("hello world!"),
				},
				Return: []byte("super random"),
			},
			{
				On: RandomnessRule{
					Kind:                RandomnessChain,
					DomainSeparationTag: 99,
					Epoch:               68592,
					Entropy:             nil, // no entropy
				},
				Return: []byte("another random value"),
			},
		},
	}

	serialized, err := json.Marshal(tv1)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(string(serialized))

	var tv2 TestVector
	err = json.Unmarshal(serialized, &tv2)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(tv1.Randomness, tv2.Randomness) {
		t.Fatal("values not equal")
	}

}

func GetInt64Keys(m *adt.Map) (out []int64) {
	var amt_val cbg.CborCid
	m.ForEach(&amt_val, func(key string) error {
		k, _ := abi.ParseIntKey(key)
		out = append(out, k)
		return nil
	})
	return out
}

/*
 * The test shows a problem with incorrect order in HAMT elements in some test vectors.
 * Here HAMT of cron events of power actor state reordered by deleting and adding the same element.
 * Reordering changes the root CID of HAMT which leads to the incorrect state of the actor.
 *
 * The same problems were found in:
 * - extracted/0004-coverage-boost/fil_1_storagepower/CreateMiner/Ok/ext-0004-fil_1_storagepower-CreateMiner-Ok-6.json
 * - extracted/0004-coverage-boost/fil_1_storagepower/CreateMiner/Ok/ext-0004-fil_1_storagepower-CreateMiner-Ok-10.json
 * - extracted/0001-initial-extraction/fil_1_storagepower/CreateMiner/Ok/ext-0001-fil_1_storagepower-CreateMiner-Ok-6.json
 */
func TestHamtOrder(t *testing.T) {
	path := "../corpus/extracted/0004-coverage-boost/fil_1_storagepower/CreateMiner/Ok/ext-0004-fil_1_storagepower-CreateMiner-Ok-6.json"
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read test file: %s", path)
	}
	var vector TestVector
	json.Unmarshal(raw, &vector)

	buf := bytes.NewReader(vector.CAR)
	store := blockstore.NewTemporary()
	gr, _ := gzip.NewReader(buf)
	defer gr.Close()

	car.LoadCar(store, gr)
	cborstore := cbor.NewCborStore(store)

	// load cron events from power actor HAMT[ChainEpoch]AMT[CronEvent] (aka HAMT[ChainEpoch]Cid)
	root_cid, _ := cid.Parse("bafy2bzaceaozxyryfjmogypgxet274qymzxeuvyy7zkhtdx5iyulrm4qfg34e")

	ctx := context.Background()
	adt_store := adt.WrapStore(ctx, cborstore)
	m, _ := adt.AsMap(adt_store, root_cid)

	prev_keys := GetInt64Keys(m)
	previous_root, _ := m.Root()

	// existing element in wrong position
	key := 56592
	var amt_cid cbg.CborCid
	// save value
	m.Get(abi.IntKey(int64(key)), &amt_cid)
	// delete element
	m.Delete(abi.IntKey(int64(key)))
	// put again - now in correct position
	m.Put(abi.IntKey(int64(key)), &amt_cid)

	new_keys := GetInt64Keys(m)
	new_root, _ := m.Root()

	fmt.Printf("Keys before: %v\n", prev_keys)
	fmt.Printf("Keys after:  %v\n", new_keys)

	fmt.Println("Prev root: ", previous_root)
	fmt.Println("New root:  ", new_root)

	require.Equal(t, prev_keys, new_keys)
	// root has been change due to reordering
	require.Equal(t, previous_root, new_root)
}
