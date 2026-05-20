package profilegen

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"noco-path-opener/internal/config"
)

type Generator struct {
	In           io.Reader
	PromptOut    io.Writer
	LocalFields  FieldLister
	RemoteFields FieldLister
}

func (g Generator) Generate(ctx context.Context) (config.SyncProfile, error) {
	in := g.In
	if in == nil {
		in = strings.NewReader("")
	}
	out := g.PromptOut
	if out == nil {
		out = io.Discard
	}

	prompt := newPrompter(in, out)
	return generateProfile(ctx, prompt, g.LocalFields, g.RemoteFields)
}

func generateProfile(ctx context.Context, prompt *prompter, localLister FieldLister, remoteLister FieldLister) (config.SyncProfile, error) {
	if localLister == nil {
		return config.SyncProfile{}, fmt.Errorf("local metadata client is not configured")
	}
	if remoteLister == nil {
		return config.SyncProfile{}, fmt.Errorf("remote metadata client is not configured")
	}

	name, err := prompt.required("Profile name: ")
	if err != nil {
		return config.SyncProfile{}, err
	}
	localBaseID, err := prompt.required("Local base_id: ")
	if err != nil {
		return config.SyncProfile{}, err
	}
	localTableID, err := prompt.required("Local table_id: ")
	if err != nil {
		return config.SyncProfile{}, err
	}
	remoteBaseID, err := prompt.required("Remote base_id: ")
	if err != nil {
		return config.SyncProfile{}, err
	}
	remoteTableID, err := prompt.required("Remote table_id: ")
	if err != nil {
		return config.SyncProfile{}, err
	}

	localFields, err := localLister.ListFields(ctx, localBaseID, localTableID)
	if err != nil {
		return config.SyncProfile{}, fmt.Errorf("fetch local fields: %w", err)
	}
	if err := requireUsableFields("local", localFields); err != nil {
		return config.SyncProfile{}, err
	}

	remoteFields, err := remoteLister.ListFields(ctx, remoteBaseID, remoteTableID)
	if err != nil {
		return config.SyncProfile{}, fmt.Errorf("fetch remote fields: %w", err)
	}
	if err := requireUsableFields("remote", remoteFields); err != nil {
		return config.SyncProfile{}, err
	}

	if err := printFieldList(prompt.out, "Local", localFields); err != nil {
		return config.SyncProfile{}, err
	}
	localLookupField, err := promptSingleField(prompt, "Local lookup field number: ", localFields)
	if err != nil {
		return config.SyncProfile{}, err
	}

	if err := printFieldList(prompt.out, "Remote", remoteFields); err != nil {
		return config.SyncProfile{}, err
	}
	remoteLookupField, err := promptSingleField(prompt, "Remote lookup field number: ", remoteFields)
	if err != nil {
		return config.SyncProfile{}, err
	}

	syncFields, err := promptMultiFields(prompt, "Sync field numbers from remote table (blank = all): ", remoteFields)
	if err != nil {
		return config.SyncProfile{}, err
	}
	if missing := missingLocalFields(syncFields, localFields); len(missing) > 0 {
		return config.SyncProfile{}, fmt.Errorf("selected sync fields do not exist in local table: %s", strings.Join(missing, ", "))
	}

	return config.SyncProfile{
		Name:              name,
		LocalBaseID:       localBaseID,
		LocalTableID:      localTableID,
		LocalLookupField:  localLookupField,
		RemoteBaseID:      remoteBaseID,
		RemoteTableID:     remoteTableID,
		RemoteLookupField: remoteLookupField,
		SyncFields:        syncFields,
	}, nil
}

type prompter struct {
	reader *bufio.Reader
	out    io.Writer
}

func newPrompter(in io.Reader, out io.Writer) *prompter {
	if in == nil {
		in = strings.NewReader("")
	}
	if out == nil {
		out = io.Discard
	}
	return &prompter{
		reader: bufio.NewReader(in),
		out:    out,
	}
}

func (p *prompter) required(label string) (string, error) {
	value, err := p.read(label)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", promptLabelName(label))
	}
	return value, nil
}

func (p *prompter) read(label string) (string, error) {
	if _, err := fmt.Fprint(p.out, label); err != nil {
		return "", err
	}

	line, err := p.reader.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return "", fmt.Errorf("read %s: %w", promptLabelName(label), err)
	}

	return strings.TrimSpace(line), nil
}

func promptLabelName(label string) string {
	name := strings.TrimSpace(label)
	name = strings.TrimSuffix(name, ":")
	return name
}

func requireUsableFields(label string, fields []Field) error {
	if len(fields) == 0 {
		return fmt.Errorf("%s table returned no usable fields", label)
	}
	for i, field := range fields {
		if field.DisplayName() == "" {
			return fmt.Errorf("%s field %d has no usable name", label, i+1)
		}
	}
	return nil
}

func printFieldList(w io.Writer, label string, fields []Field) error {
	if _, err := fmt.Fprintf(w, "%s fields:\n", label); err != nil {
		return err
	}
	for i, field := range fields {
		if _, err := fmt.Fprintf(w, "  %d. %s\n", i+1, field.DisplayName()); err != nil {
			return err
		}
	}
	return nil
}

func promptSingleField(prompt *prompter, label string, fields []Field) (string, error) {
	input, err := prompt.required(label)
	if err != nil {
		return "", err
	}
	index, err := parseSingleSelection(input, len(fields))
	if err != nil {
		return "", fmt.Errorf("%s: %w", promptLabelName(label), err)
	}
	return fields[index].DisplayName(), nil
}

func promptMultiFields(prompt *prompter, label string, fields []Field) ([]string, error) {
	input, err := prompt.read(label)
	if err != nil {
		return nil, err
	}
	if input == "" {
		return fieldNames(fields), nil
	}
	indexes, err := parseMultiSelection(input, len(fields))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", promptLabelName(label), err)
	}

	names := make([]string, 0, len(indexes))
	for _, index := range indexes {
		names = append(names, fields[index].DisplayName())
	}
	return names, nil
}

func fieldNames(fields []Field) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.DisplayName())
	}
	return names
}

func missingLocalFields(remoteNames []string, localFields []Field) []string {
	localNames := make(map[string]struct{}, len(localFields))
	for _, field := range localFields {
		localNames[field.DisplayName()] = struct{}{}
	}

	missing := make([]string, 0)
	for _, name := range remoteNames {
		if _, exists := localNames[name]; !exists {
			missing = append(missing, name)
		}
	}
	return missing
}
