package secrets_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/secrets"
	"go.uber.org/mock/gomock"
	"pgregory.net/rapid"
)

// asOp is the variable that makes this test binary answer as op, so
// OnePassword's own reading is tested without 1Password.
const asOp = "SECRETS_TEST_AS_OP"

// What the stand-in op prints: a secret for any field but nope, and for
// nope a value on its output and a refusal on its errors.
const (
	opSecret  = "s3cret-value"
	opPrinted = "printed-before-refusing"
	opRefusal = "[ERROR] no such item"
)

// The codes the stand-in op exits with.
const (
	opOK      = 0 // defined by the shell: success
	opRefused = 1 // defined by the shell: a failure
)

// afterName is where the words after the command's name begin. It's
// defined by os.Args.
const afterName = 1

// TestMain runs the tests, or answers as op when asOp says so.
func TestMain(suite *testing.M) {
	if os.Getenv(asOp) != "" {
		os.Exit(answerAsOp(os.Args[afterName:]))
	}
	os.Exit(suite.Run())
}

// answerAsOp is op's answer to op read REFERENCE: the secret; a
// refusal with words for a field named nope; or one with no words for a
// field named quiet.
func answerAsOp(words []string) int {
	asked := strings.Join(words, " ")
	if strings.HasSuffix(asked, "/quiet") {
		return opRefused
	}
	if !strings.HasPrefix(asked, "read ") ||
		!strings.HasSuffix(asked, "/nope") {
		fmt.Println(opSecret)
		return opOK
	}
	fmt.Println(opPrinted)
	fmt.Fprintln(os.Stderr, opRefusal)
	return opRefused
}

// self is this test binary, which answers as op.
func self(t *testing.T) string {
	t.Helper()
	t.Setenv(asOp, "1")
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// The items the tests place secrets in.
const (
	adsItem  = "op://Vault/ads token"
	gaiaItem = "op://Vault/gaia login"
)

// configWith is a config of these lines, written in a test's folder.
func configWith(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	text := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(text), configMode); err != nil {
		t.Fatal(err)
	}
	return path
}

// configMode is the mode a test's config is written with. It's defined:
// read and written by its owner alone.
const configMode = 0o600

// opened is the keeper of a config, reading through keys, or the test
// stops.
func opened(t *testing.T, path string, keys secrets.Reader) *secrets.Keeper {
	t.Helper()
	keeper, err := secrets.Open(path, keys)
	if err != nil {
		t.Fatal(err)
	}
	return keeper
}

// The fields each service's secret has.
var (
	adsFields  = []string{"credential"}
	gaiaFields = []string{"username", "password"}
)

// fieldsOf is each service's fields, by name.
var fieldsOf = map[string][]string{"ads": adsFields, "gaia": gaiaFields}

// the test: every service says its name, what it is and its fields, in
// order: ads, then gaia.
func TestEveryServiceSaysItsFields(t *testing.T) {
	var said []string
	for _, service := range secrets.Known() {
		said = append(said, service.Name())
		want := fieldsOf[service.Name()]
		if !slices.Equal(service.Fields(), want) || service.About() == "" {
			t.Errorf("%s: %v, %q", service.Name(), service.Fields(),
				service.About())
		}
	}
	if !slices.Equal(said, []string{"ads", "gaia"}) {
		t.Errorf("%v", said)
	}
}

// the anti-test: a service's fields can't be changed by whoever is given
// them, through Known or Fields; the secret read is still each field.
func TestAServicesFieldsAreNotChangedFromOutside(t *testing.T) {
	for _, service := range secrets.Known() {
		clear(service.Fields())
		clear(service.Fields())
	}
	for _, service := range secrets.Known() {
		if want := fieldsOf[service.Name()]; !slices.Equal(service.Fields(),
			want) {
			t.Errorf("%s: %v", service.Name(), service.Fields())
		}
	}
}

// the anti-test: a name no service has is refused, naming the services
// there are, and its secret is not read.
func TestAnUnknownServiceIsRefusedNamingTheKnownOnes(t *testing.T) {
	keys := mocks.NewMockReader(gomock.NewController(t))
	keeper := opened(t, configWith(t, "secret ads :: "+adsItem), keys)
	_, itemErr := keeper.Item("hubble")
	secret, err := keeper.Secret("hubble")
	for _, refused := range []error{itemErr, err} {
		if refused == nil || !strings.Contains(refused.Error(),
			`no such service: "hubble"; celestial-data signs in to `+
				"ads and gaia") {
			t.Errorf("%v", refused)
		}
	}
	if secret != nil {
		t.Errorf("%v", secret)
	}
}

// the test: the config is the tool's own, in its folder of the home's
// .config, as pzm keeps every tool's.
func TestTheConfigIsTheToolsOwn(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, ".config", "celestial-data-secrets",
		"config")
	if got := secrets.Path(home); got != want || secrets.Tool !=
		"celestial-data-secrets" {
		t.Errorf("%s, %s", got, secrets.Tool)
	}
}

// the anti-test: two homes keep two configs.
func TestTwoHomesKeepTwoConfigs(t *testing.T) {
	one, other := t.TempDir(), t.TempDir()
	if secrets.Path(one) == secrets.Path(other) {
		t.Error("two homes share a config")
	}
}

// the test: the config places each service's secret at its item, with
// comments and blank lines between.
func TestTheConfigPlacesEachService(t *testing.T) {
	path := configWith(t, "# where the secrets are", "",
		"secret ads :: "+adsItem, "secret gaia :: "+gaiaItem)
	keeper := opened(t, path, nil)
	for name, want := range map[string]string{"ads": adsItem,
		"gaia": gaiaItem} {
		if item, err := keeper.Item(name); err != nil || item != want {
			t.Errorf("%s: %q, %v", name, item, err)
		}
	}
}

// the anti-test: with no config, nothing is placed, and asking for a
// service says the line to add and where.
func TestAMissingConfigPlacesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "none")
	_, err := opened(t, path, nil).Item("ads")
	if !errors.Is(err, secrets.Unplaced()) || !strings.Contains(err.Error(),
		`for ads in `+path+`: add a line "secret ads :: op://vault/item"`) {
		t.Errorf("%v", err)
	}
}

// the anti-test: a line whose tail is not an item as op names one is
// refused, naming its line; so is a line of no tail.
func TestALineThatIsNotAnItemIsRefused(t *testing.T) {
	for _, tail := range []string{"op://Vault", "https://Vault/item",
		"op://Vault/item/field", "op:///item", "op://Vault/", "op://",
		"Vault/item", "op://  /item"} {
		path := configWith(t, "secret gaia :: "+gaiaItem,
			"secret ads :: "+tail)
		_, err := secrets.Open(path, nil)
		if err == nil || !strings.Contains(err.Error(), ":2: secret: ") ||
			!strings.Contains(err.Error(), "is not an item as op names") {
			t.Errorf("%q: %v", tail, err)
		}
	}
}

// the anti-test: a line for a service celestial-data does not sign in to
// is refused.
func TestALineForAnUnknownServiceIsRefused(t *testing.T) {
	_, err := secrets.Open(configWith(t, "secret hubble :: "+adsItem), nil)
	if err == nil || !strings.Contains(err.Error(), `"hubble"`) {
		t.Errorf("%v", err)
	}
}

// the anti-test: a service placed twice is refused, naming both lines.
func TestAServicePlacedTwiceIsRefused(t *testing.T) {
	path := configWith(t, "secret ads :: "+adsItem, "secret gaia :: "+
		gaiaItem, "secret ads :: "+gaiaItem)
	_, err := secrets.Open(path, nil)
	if err == nil || !strings.Contains(err.Error(), path+":3: secret: ads "+
		"is placed at line 1 already") {
		t.Errorf("%v", err)
	}
}

// the anti-test: a config that can't be read is refused, not taken for
// none.
func TestAConfigThatWillNotReadIsRefused(t *testing.T) {
	if _, err := secrets.Open(t.TempDir(), nil); err == nil {
		t.Error("a folder was read as a config")
	}
}

// the test: a secret is each of its fields, read from its item in the
// order the service names them.
func TestASecretIsEachOfItsFields(t *testing.T) {
	keys := mocks.NewMockReader(gomock.NewController(t))
	gomock.InOrder(
		keys.EXPECT().Read(gaiaItem+"/username").Return("ada", nil),
		keys.EXPECT().Read(gaiaItem+"/password").Return("pw", nil))
	path := configWith(t, "secret gaia :: "+gaiaItem)
	got, err := opened(t, path, keys).Secret("gaia")
	want := map[string]string{"username": "ada", "password": "pw"}
	if err != nil || len(got) != len(want) || got["username"] != "ada" ||
		got["password"] != "pw" {
		t.Errorf("%v, %v", got, err)
	}
}

// the anti-test: a field op will not give stops the secret, naming the
// service and the field, and gives back no part of it.
func TestAFieldOpWillNotGiveStopsTheSecret(t *testing.T) {
	keys := mocks.NewMockReader(gomock.NewController(t))
	refused := errors.New("op said no")
	keys.EXPECT().Read(gaiaItem+"/username").Return("ada", nil)
	keys.EXPECT().Read(gaiaItem+"/password").Return("", refused)
	path := configWith(t, "secret gaia :: "+gaiaItem)
	got, err := opened(t, path, keys).Secret("gaia")
	if got != nil || !errors.Is(err, refused) ||
		!strings.Contains(err.Error(), "gaia's password") ||
		strings.Contains(err.Error(), "ada") {
		t.Errorf("%v, %v", got, err)
	}
}

// the anti-test: a service the config does not place asks op nothing.
func TestAnUnplacedSecretAsksOpNothing(t *testing.T) {
	keys := mocks.NewMockReader(gomock.NewController(t))
	path := configWith(t, "secret gaia :: "+gaiaItem)
	if _, err := opened(t, path, keys).Secret("ads"); !errors.Is(err,
		secrets.Unplaced()) {
		t.Errorf("%v", err)
	}
}

// the test: OnePassword gives what op prints for a reference, without
// the end of its line.
func TestOnePasswordReadsWhatOpPrints(t *testing.T) {
	got, err := secrets.NewOnePassword(self(t)).Read(adsItem + "/credential")
	if err != nil || got != opSecret {
		t.Errorf("%q, %v", got, err)
	}
}

// the anti-test: op's refusal is an error that says what op said and
// the reference, and never what op printed.
func TestOnePasswordSaysWhenOpRefuses(t *testing.T) {
	got, err := secrets.NewOnePassword(self(t)).Read(adsItem + "/nope")
	if got != "" || err == nil || !strings.Contains(err.Error(), opRefusal) ||
		!strings.Contains(err.Error(), adsItem+"/nope") ||
		strings.Contains(err.Error(), opPrinted) {
		t.Errorf("%q, %v", got, err)
	}
}

// the anti-test: a refusal op gives no words for ends with the refusal,
// not with an empty place for words.
func TestOnePasswordSaysASilentRefusalPlainly(t *testing.T) {
	_, err := secrets.NewOnePassword(self(t)).Read(adsItem + "/quiet")
	if err == nil || strings.HasSuffix(err.Error(), ": ") ||
		!strings.HasSuffix(err.Error(), "exit status 1") {
		t.Errorf("%v", err)
	}
}

// the property: any item named in a line of the config is the item the
// keeper gives for its service, whatever its vault and item are called.
func TestAnyPlacedItemIsGivenBack(t *testing.T) {
	rapid.Check(t, placing{t}.givenBack)
}

// placing is the property's test, whose folders the configs are written
// in.
type placing struct{ t *testing.T }

// givenBack draws a service and an item, places it, and holds the item
// the keeper gives to the one placed.
func (property placing) givenBack(draw *rapid.T) {
	name := rapid.SampledFrom([]string{"ads", "gaia"}).Draw(draw,
		"service")
	vault := rapid.StringMatching(itemName).Draw(draw, "vault")
	item := rapid.StringMatching(itemName).Draw(draw, "item")
	placed := "op://" + vault + "/" + item
	path := configWith(property.t, "secret "+name+" :: "+placed)
	got, err := opened(property.t, path, nil).Item(name)
	if err != nil || got != placed {
		draw.Fatalf("%q: %q, %v", placed, got, err)
	}
}

// itemName is a vault's or an item's name the property draws: words with
// no slash, that start and end with a letter or a digit.
const itemName = `[A-Za-z0-9]([A-Za-z0-9 ._-]{0,30}[A-Za-z0-9])?`
