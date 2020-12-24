package message

import "bytes"

type MessageCodec interface {
	EncodePingMessage(descriptorHeader DescriptorHeader) ([]byte, error)
	EncodePongMessage(descriptorHeader DescriptorHeader, pongMessage PongMessage) ([]byte, error)
	EncodeQueryMessage(descriptorHeader DescriptorHeader, queryMessage QueryMessage) ([]byte, error)
	EncodeQueryHitMessage(descriptorHeader DescriptorHeader, queryHitMessage QueryHitMessage) ([]byte, error)

	DecodeDescriptorHeader(buffer *bytes.Buffer) (DescriptorHeader, error)
	DecodePongMessage(buffer *bytes.Buffer) (PongMessage, error)
	DecodeQueryMessage(buffer *bytes.Buffer) (QueryMessage, error)
	DecodeQueryHitMessage(buffer *bytes.Buffer) (QueryHitMessage, error)
}
