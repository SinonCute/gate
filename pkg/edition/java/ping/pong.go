package ping

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gate/pkg/edition/java/proto/util"
	"gate/pkg/gate/proto"
	"gate/pkg/util/componentutil"
	"gate/pkg/util/favicon"
	"gate/pkg/util/uuid"

	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/common/minecraft/component/codec/legacy"
	"gopkg.in/yaml.v3"
)

// ServerPing is a 1.7 and above server list ping response.
type ServerPing struct {
	Version     Version         `json:"version,omitempty" yaml:"version,omitempty"`
	Players     *Players        `json:"players,omitempty" yaml:"players,omitempty"`
	Description *component.Text `json:"description" yaml:"description"`
	Favicon     favicon.Favicon `json:"favicon,omitempty" yaml:"favicon,omitempty"`
}

// Make sure ServerPing implements the interfaces at compile time.
var (
	_ json.Marshaler   = (*ServerPing)(nil)
	_ json.Unmarshaler = (*ServerPing)(nil)

	_ yaml.Marshaler   = (*ServerPing)(nil)
	_ yaml.Unmarshaler = (*ServerPing)(nil)
)

func (p *ServerPing) MarshalJSON() ([]byte, error) {
	b := new(bytes.Buffer)
	err := util.JsonCodec(p.Version.Protocol).Marshal(b, p.Description)
	if err != nil {
		return nil, err
	}

	type Alias ServerPing
	return json.Marshal(&struct {
		Description json.RawMessage `json:"description"`
		*Alias
	}{
		Description: b.Bytes(),
		Alias:       (*Alias)(p),
	})
}
func (p *ServerPing) UnmarshalJSON(data []byte) error {
	type Alias ServerPing
	out := &struct {
		Alias
		Description json.RawMessage `json:"description"` // override description type
	}{}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("error decoding json: %w", err)
	}

	var err error
	out.Alias.Description, err = componentutil.ParseTextComponent(out.Version.Protocol, string(out.Description))
	if err != nil {
		return fmt.Errorf("error decoding description: %w", err)
	}

	*p = ServerPing(out.Alias)
	return nil
}

func (p *ServerPing) UnmarshalYAML(value *yaml.Node) error {
	type Alias ServerPing
	out := &struct {
		Description string `yaml:"description"` // override description type
		*Alias
	}{
		Alias: (*Alias)(p),
	}
	if err := value.Decode(out); err != nil {
		return fmt.Errorf("error decoding yaml: %w", err)
	}

	var err error
	p.Description, err = componentutil.ParseTextComponent(out.Version.Protocol, out.Description)
	if err != nil {
		return fmt.Errorf("error decoding description: %w", err)
	}

	*p = ServerPing(*out.Alias)
	return nil
}

func (p *ServerPing) MarshalYAML() (interface{}, error) {
	b := new(strings.Builder)
	err := (&legacy.Legacy{}).Marshal(b, p.Description)
	if err != nil {
		return nil, fmt.Errorf("error encoding description: %w", err)
	}

	type Alias ServerPing
	return &struct {
		Description string
		*Alias
	}{
		Description: b.String(),
		Alias:       (*Alias)(p),
	}, nil
}

type Version struct {
	Protocol proto.Protocol `json:"protocol,omitempty" yaml:"protocol,omitempty"`
	Name     string         `json:"name,omitempty" yaml:"name,omitempty"`
}

type Players struct {
	Online int            `json:"online"`
	Max    int            `json:"max"`
	Sample []SamplePlayer `json:"sample,omitempty"`
}

type SamplePlayer struct {
	Name string    `json:"name"`
	ID   uuid.UUID `json:"id"`
}
