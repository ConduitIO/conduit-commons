// Copyright © 2023 Meroxa, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package opencdc

import (
	"fmt"
	"strconv"
	"time"
)

const (
	// OpenCDCVersion is a constant that should be used as the value in the
	// metadata field MetadataVersion. It ensures the OpenCDC format version can
	// be easily identified in case the record gets marshaled into a different
	// untyped format (e.g. JSON).
	OpenCDCVersion = "v1"

	// MetadataOpenCDCVersion is a Record.Metadata key for the version of the
	// OpenCDC format (e.g. "v1"). This field exists to ensure the OpenCDC
	// format version can be easily identified in case the record gets marshaled
	// into a different untyped format (e.g. JSON).
	MetadataOpenCDCVersion = "opencdc.version"
	// MetadataCreatedAt is a Record.Metadata key for the time when the record
	// was created in the 3rd party system. The expected format is a unix
	// timestamp in nanoseconds.
	MetadataCreatedAt = "opencdc.createdAt"
	// MetadataReadAt is a Record.Metadata key for the time when the record was
	// read from the 3rd party system. The expected format is a unix timestamp
	// in nanoseconds.
	MetadataReadAt = "opencdc.readAt"
	// MetadataCollection is a Record.Metadata key for the name of the collection
	// where the record originated from and/or where it should be stored.
	MetadataCollection = "opencdc.collection"

	// MetadataKeySchemaName is a Record.Metadata key for the name of the schema of
	// the record's .Key field.
	MetadataKeySchemaName = "opencdc.key.schema.name"
	// MetadataKeySchemaVersion is a Record.Metadata key for the version of the schema of
	// the record's .Key field.
	MetadataKeySchemaVersion = "opencdc.key.schema.version"
	// MetadataKeySchemaType is a Record.Metadata key for the type of the schema of
	// the record's .Key field.
	MetadataKeySchemaType = "opencdc.key.schema.type"

	// MetadataPayloadSchemaName is a Record.Metadata key for the name of the schema of
	// the record's .Payload field.
	MetadataPayloadSchemaName = "opencdc.payload.schema.name"
	// MetadataPayloadSchemaVersion is a Record.Metadata key for the version of the schema of
	// the record's .Payload field.
	MetadataPayloadSchemaVersion = "opencdc.payload.schema.version"
	// MetadataPayloadSchemaType is a Record.Metadata key for the type of the schema of
	// the record's .Payload field.
	MetadataPayloadSchemaType = "opencdc.payload.schema.type"

	// MetadataConduitSourcePluginName is a Record.Metadata key for the name of
	// the source plugin that created this record.
	MetadataConduitSourcePluginName = "conduit.source.plugin.name"
	// MetadataConduitSourcePluginVersion is a Record.Metadata key for the
	// version of the source plugin that created this record.
	MetadataConduitSourcePluginVersion = "conduit.source.plugin.version"
	// MetadataConduitDestinationPluginName is a Record.Metadata key for the
	// name of the destination plugin that has written this record
	// (only available in records once they are written by a destination).
	MetadataConduitDestinationPluginName = "conduit.destination.plugin.name"
	// MetadataConduitDestinationPluginVersion is a Record.Metadata key for the
	// version of the destination plugin that has written this record
	// (only available in records once they are written by a destination).
	MetadataConduitDestinationPluginVersion = "conduit.destination.plugin.version"

	// MetadataConduitSourceConnectorID is a Record.Metadata key for the ID of
	// the source connector that produced this record.
	MetadataConduitSourceConnectorID = "conduit.source.connector.id"
	// MetadataConduitDLQNackError is a Record.Metadata key for the error that
	// caused a record to be nacked and pushed to the dead-letter queue.
	MetadataConduitDLQNackError = "conduit.dlq.nack.error"
	// MetadataConduitDLQNackNodeID is a Record.Metadata key for the ID of the
	// internal node that nacked the record.
	MetadataConduitDLQNackNodeID = "conduit.dlq.nack.node.id"
)

type Metadata map[string]string

// SetOpenCDCVersion sets the metadata value for key MetadataVersion to the
// current version of OpenCDC used.
func (m Metadata) SetOpenCDCVersion() {
	m[MetadataOpenCDCVersion] = OpenCDCVersion
}

// GetOpenCDCVersion returns the value for key
// MetadataOpenCDCVersion. If the value does not exist or is empty the
// function returns ErrMetadataFieldNotFound.
func (m Metadata) GetOpenCDCVersion() (string, error) {
	return m.getValue(MetadataOpenCDCVersion)
}

// GetCreatedAt parses the value for key MetadataCreatedAt as a unix
// timestamp. If the value does not exist or the value is empty the function
// returns ErrMetadataFieldNotFound. If the value is not a valid unix timestamp
// in nanoseconds the function returns an error.
func (m Metadata) GetCreatedAt() (time.Time, error) {
	raw, err := m.getValue(MetadataCreatedAt)
	if err != nil {
		return time.Time{}, err
	}

	unixNano, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse value for %q: %w", MetadataCreatedAt, err)
	}

	return time.Unix(0, unixNano), nil
}

// SetCreatedAt sets the metadata value for key MetadataCreatedAt as a
// unix timestamp in nanoseconds.
func (m Metadata) SetCreatedAt(createdAt time.Time) {
	m[MetadataCreatedAt] = strconv.FormatInt(createdAt.UnixNano(), 10)
}

// GetReadAt parses the value for key MetadataReadAt as a unix
// timestamp. If the value does not exist or the value is empty the function
// returns ErrMetadataFieldNotFound. If the value is not a valid unix timestamp
// in nanoseconds the function returns an error.
func (m Metadata) GetReadAt() (time.Time, error) {
	raw, err := m.getValue(MetadataReadAt)
	if err != nil {
		return time.Time{}, err
	}

	unixNano, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse value for %q: %w", MetadataReadAt, err)
	}

	return time.Unix(0, unixNano), nil
}

// SetReadAt sets the metadata value for key MetadataReadAt as a unix
// timestamp in nanoseconds.
func (m Metadata) SetReadAt(createdAt time.Time) {
	m[MetadataReadAt] = strconv.FormatInt(createdAt.UnixNano(), 10)
}

// GetCollection returns the value for key MetadataCollection. If the value does
// not exist or is empty the function returns ErrMetadataFieldNotFound.
func (m Metadata) GetCollection() (string, error) {
	return m.getValue(MetadataCollection)
}

// SetCollection sets the metadata value for key MetadataCollection.
func (m Metadata) SetCollection(collection string) {
	m[MetadataCollection] = collection
}

// GetConduitSourcePluginName returns the value for key
// MetadataConduitSourcePluginName. If the value does not exist or is empty the
// function returns ErrMetadataFieldNotFound.
func (m Metadata) GetConduitSourcePluginName() (string, error) {
	return m.getValue(MetadataConduitSourcePluginName)
}

// SetConduitSourcePluginName sets the metadata value for key
// MetadataConduitSourcePluginName.
func (m Metadata) SetConduitSourcePluginName(name string) {
	m[MetadataConduitSourcePluginName] = name
}

// GetConduitSourcePluginVersion returns the value for key
// MetadataConduitSourcePluginVersion. If the value does not exist or is empty
// the function returns ErrMetadataFieldNotFound.
func (m Metadata) GetConduitSourcePluginVersion() (string, error) {
	return m.getValue(MetadataConduitSourcePluginVersion)
}

// SetConduitSourcePluginVersion sets the metadata value for key
// MetadataConduitSourcePluginVersion.
func (m Metadata) SetConduitSourcePluginVersion(version string) {
	m[MetadataConduitSourcePluginVersion] = version
}

// GetConduitDestinationPluginName returns the value for key
// MetadataConduitDestinationPluginName. If the value does not exist or is empty
// the function returns ErrMetadataFieldNotFound.
func (m Metadata) GetConduitDestinationPluginName() (string, error) {
	return m.getValue(MetadataConduitDestinationPluginName)
}

// SetConduitDestinationPluginName sets the metadata value for key
// MetadataConduitDestinationPluginName.
func (m Metadata) SetConduitDestinationPluginName(name string) {
	m[MetadataConduitDestinationPluginName] = name
}

// GetConduitDestinationPluginVersion returns the value for key
// MetadataConduitDestinationPluginVersion. If the value does not exist or is
// empty the function returns ErrMetadataFieldNotFound.
func (m Metadata) GetConduitDestinationPluginVersion() (string, error) {
	return m.getValue(MetadataConduitDestinationPluginVersion)
}

// SetConduitDestinationPluginVersion sets the metadata value for key
// MetadataConduitDestinationPluginVersion.
func (m Metadata) SetConduitDestinationPluginVersion(version string) {
	m[MetadataConduitDestinationPluginVersion] = version
}

// GetConduitSourceConnectorID returns the value for key
// MetadataConduitSourceConnectorID. If the value does not exist or is empty the
// function returns ErrMetadataFieldNotFound.
func (m Metadata) GetConduitSourceConnectorID() (string, error) {
	return m.getValue(MetadataConduitSourceConnectorID)
}

// SetConduitSourceConnectorID sets the metadata value for key
// MetadataConduitSourceConnectorID.
func (m Metadata) SetConduitSourceConnectorID(id string) {
	m[MetadataConduitSourceConnectorID] = id
}

// GetConduitDLQNackError returns the value for key
// MetadataConduitDLQNackError. If the value does not exist or is empty the
// function returns ErrMetadataFieldNotFound.
func (m Metadata) GetConduitDLQNackError() (string, error) {
	return m.getValue(MetadataConduitDLQNackError)
}

// SetConduitDLQNackError sets the metadata value for key
// MetadataConduitDLQNackError.
func (m Metadata) SetConduitDLQNackError(err string) {
	m[MetadataConduitDLQNackError] = err
}

// GetConduitDLQNackNodeID returns the value for key
// MetadataConduitDLQNackNodeID. If the value does not exist or is empty the
// function returns ErrMetadataFieldNotFound.
func (m Metadata) GetConduitDLQNackNodeID() (string, error) {
	return m.getValue(MetadataConduitDLQNackNodeID)
}

// SetConduitDLQNackNodeID sets the metadata value for key
// MetadataConduitDLQNackNodeID.
func (m Metadata) SetConduitDLQNackNodeID(id string) {
	m[MetadataConduitDLQNackNodeID] = id
}

// GetKeySchemaName returns the value for key MetadataKeySchemaName.
// If the value does not exist or is empty the function returns ErrMetadataFieldNotFound.
func (m Metadata) GetKeySchemaName() (string, error) {
	return m.getValue(MetadataKeySchemaName)
}

// SetKeySchemaName sets the metadata value for key MetadataKeySchemaName.
func (m Metadata) SetKeySchemaName(name string) {
	m[MetadataKeySchemaName] = name
}

// GetKeySchemaVersion returns the value for key MetadataKeySchemaVersion.
// If the value does not exist or is empty the function returns ErrMetadataFieldNotFound.
func (m Metadata) GetKeySchemaVersion() (int, error) {
	vs, err := m.getValue(MetadataKeySchemaVersion)
	if err != nil {
		return 0, err
	}

	v, err := strconv.Atoi(vs)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", vs, err)
	}

	return v, nil
}

// SetKeySchemaVersion sets the metadata value for key MetadataKeySchemaVersion.
func (m Metadata) SetKeySchemaVersion(version int) {
	m[MetadataKeySchemaVersion] = strconv.Itoa(version)
}

// GetKeySchemaType returns the value for key MetadataKeySchemaType.
// If the value does not exist or is empty the function returns ErrMetadataFieldNotFound.
func (m Metadata) GetKeySchemaType() (string, error) {
	return m.getValue(MetadataKeySchemaType)
}

// SetKeySchemaType sets the metadata value for key MetadataKeySchemaType.
func (m Metadata) SetKeySchemaType(t string) {
	m[MetadataKeySchemaType] = t
}

// GetPayloadSchemaName returns the value for key MetadataPayloadSchemaName.
// If the value does not exist or is empty the function returns ErrMetadataFieldNotFound.
func (m Metadata) GetPayloadSchemaName() (string, error) {
	return m.getValue(MetadataPayloadSchemaName)
}

// SetPayloadSchemaName sets the metadata value for key MetadataPayloadSchemaName.
func (m Metadata) SetPayloadSchemaName(name string) {
	m[MetadataPayloadSchemaName] = name
}

// GetPayloadSchemaVersion returns the value for key MetadataPayloadSchemaVersion.
// If the value does not exist or is empty the function returns ErrMetadataFieldNotFound.
func (m Metadata) GetPayloadSchemaVersion() (int, error) {
	vs, err := m.getValue(MetadataPayloadSchemaVersion)
	if err != nil {
		return 0, err
	}

	v, err := strconv.Atoi(vs)
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", vs, err)
	}

	return v, nil
}

// SetPayloadSchemaVersion sets the metadata value for key MetadataPayloadSchemaVersion.
func (m Metadata) SetPayloadSchemaVersion(version int) {
	m[MetadataPayloadSchemaVersion] = strconv.Itoa(version)
}

// GetPayloadSchemaType returns the value for key MetadataPayloadSchemaType.
// If the value does not exist or is empty the function returns ErrMetadataFieldNotFound.
func (m Metadata) GetPayloadSchemaType() (string, error) {
	return m.getValue(MetadataPayloadSchemaType)
}

// SetPayloadSchemaType sets the metadata value for key MetadataPayloadSchemaType.
func (m Metadata) SetPayloadSchemaType(t string) {
	m[MetadataPayloadSchemaType] = t
}

// getValue returns the value for a specific key. If the value does not exist or
// is empty the function returns ErrMetadataFieldNotFound.
func (m Metadata) getValue(key string) (string, error) {
	str := m[key]
	if str == "" {
		return "", fmt.Errorf("failed to get value for %q: %w", key, ErrMetadataFieldNotFound)
	}
	return str, nil
}
