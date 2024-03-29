// Implementation of a forward-secure, end-to-end encrypted messaging client
// supporting key compromise recovery and out-of-order message delivery.
// Directly inspired by Signal/Double-ratchet protocol but missing a few
// features. No asynchronous handshake support (pre-keys) for example.
//
// SECURITY WARNING: This code is meant for educational purposes and may
// contain vulnerabilities or other bugs. Please do not use it for
// security-critical applications.
//
// GRADING NOTES: This is the only file you need to modify for this assignment.
// You may add additional support files if desired. You should modify this file
// to implement the intended protocol, but preserve the function signatures
// for the following methods to ensure your implementation will work with
// standard test code:
//
// *NewChatter
// *EndSession
// *InitiateHandshake
// *ReturnHandshake
// *FinalizeHandshake
// *SendMessage
// *ReceiveMessage
//
// In addition, you'll need to keep all of the following structs' fields:
//
// *Chatter
// *Session
// *Message
//
// You may add fields if needed (not necessary) but don't rename or delete
// any existing fields.
//
// Original version
// Joseph Bonneau February 2019

package chatterbox

import (
	//	"bytes" //un-comment for helpers like bytes.equal
	"encoding/binary"
	"errors"
	// "fmt" //un-comment if you want to do any debug printing.
)

// Labels for key derivation

// Label for generating a check key from the initial root.
// Used for verifying the results of a handshake out-of-band.
const HANDSHAKE_CHECK_LABEL byte = 0x11

// Label for ratcheting the root key after deriving a key chain from it
const ROOT_LABEL = 0x22

// Label for ratcheting the main chain of keys
const CHAIN_LABEL = 0x33

// Label for deriving message keys from chain keys
const KEY_LABEL = 0x44

// Chatter represents a chat participant. Each Chatter has a single long-term
// key Identity, and a map of open sessions with other users (indexed by their
// identity keys). You should not need to modify this.
type Chatter struct {
	Identity *KeyPair
	Sessions map[PublicKey]*Session
}

// Session represents an open session between one chatter and another.
// You should not need to modify this, though you can add additional fields
// if you want to.
type Session struct {
	MyDHRatchet       *KeyPair
	PartnerDHRatchet  *PublicKey
	RootChain         *SymmetricKey
	SendChain         *SymmetricKey
	ReceiveChain      *SymmetricKey
	CachedReceiveKeys map[int]*SymmetricKey
	SendCounter       int
	LastUpdate        int
	ReceiveCounter    int
	Change			  bool
}

// Message represents a message as sent over an untrusted network.
// The first 5 fields are send unencrypted (but should be authenticated).
// The ciphertext contains the (encrypted) communication payload.
// You should not need to modify this.
type Message struct {
	Sender        *PublicKey
	Receiver      *PublicKey
	NextDHRatchet *PublicKey
	Counter       int
	LastUpdate    int
	Ciphertext    []byte
	IV            []byte
	Change		  bool
}

// InitiateHandshake prepares the first message sent in a handshake, containing
// an ephemeral DH share. The partner which calls this method is the initiator.
func (c *Chatter) InitiateHandshake(partnerIdentity *PublicKey) (*PublicKey, error) {

	if _, exists := c.Sessions[*partnerIdentity]; exists {
		return nil, errors.New("Already have session open")
	}

	pair:=GenerateKeyPair()

	c.Sessions[*partnerIdentity] = &Session{
		MyDHRatchet: 	 pair,
		CachedReceiveKeys: make(map[int]*SymmetricKey),
		SendCounter:       0,
		LastUpdate:        0,
		ReceiveCounter:    0,
		Change: 		  false,
	}

	return &pair.PublicKey, nil
}

// ReturnHandshake prepares the second message sent in a handshake, containing
// an ephemeral DH share. The partner which calls this method is the responder.
func (c *Chatter) ReturnHandshake(partnerIdentity,
	partnerEphemeral *PublicKey) (*PublicKey, *SymmetricKey, error) {

	if _, exists := c.Sessions[*partnerIdentity]; exists {
		return nil, nil, errors.New("Already have session open")
	}

	pair:=GenerateKeyPair()
	root:=CombineKeys(DHCombine(partnerIdentity,&pair.PrivateKey),DHCombine(partnerEphemeral,&c.Identity.PrivateKey),DHCombine(partnerEphemeral,&pair.PrivateKey))

	c.Sessions[*partnerIdentity] = &Session{
		MyDHRatchet:       pair,
		PartnerDHRatchet:  partnerEphemeral,
		RootChain:         root,
		SendChain:         root.DeriveKey(CHAIN_LABEL),
		ReceiveChain:      root.DeriveKey(CHAIN_LABEL),
		CachedReceiveKeys: make(map[int]*SymmetricKey),
		SendCounter:       0,
		LastUpdate:        0,
		ReceiveCounter:    0,
		Change: 		  true,
	}

	return &pair.PublicKey, root.DeriveKey(HANDSHAKE_CHECK_LABEL), nil

}

// FinalizeHandshake lets the initiator receive the responder's ephemeral key
// and finalize the handshake.The partner which calls this method is the initiator.
func (c *Chatter) FinalizeHandshake(partnerIdentity,
	partnerEphemeral *PublicKey) (*SymmetricKey, error) {

	if _, exists := c.Sessions[*partnerIdentity]; !exists {
		return nil, errors.New("Can't finalize session, not yet open")
	}

	session := c.Sessions[*partnerIdentity]

	root:=CombineKeys(DHCombine(partnerEphemeral,&c.Identity.PrivateKey),DHCombine(partnerIdentity,&session.MyDHRatchet.PrivateKey),DHCombine(partnerEphemeral,&session.MyDHRatchet.PrivateKey))

	session.RootChain = root
	session.SendChain = root.DeriveKey(CHAIN_LABEL)
	session.ReceiveChain = root.DeriveKey(CHAIN_LABEL)
	session.PartnerDHRatchet = partnerEphemeral

	return root.DeriveKey(HANDSHAKE_CHECK_LABEL), nil
}

// SendMessage is used to send the given plaintext string as a message.
// You'll need to implement the code to ratchet, derive keys and encrypt this message.
func (c *Chatter) SendMessage(partnerIdentity *PublicKey,
	plaintext string) (*Message, error) {

	if _, exists := c.Sessions[*partnerIdentity]; !exists {
		return nil, errors.New("Can't send message to partner with no open session")
	}

	c.Sessions[*partnerIdentity].SendCounter++
	def:=false

	if c.Sessions[*partnerIdentity].Change{
	newKeys:=GenerateKeyPair()
	c.Sessions[*partnerIdentity].MyDHRatchet=newKeys
	root:=c.Sessions[*partnerIdentity].RootChain.DeriveKey(ROOT_LABEL)
	c.Sessions[*partnerIdentity].RootChain=CombineKeys(root,DHCombine(c.Sessions[*partnerIdentity].PartnerDHRatchet,&newKeys.PrivateKey))
	c.Sessions[*partnerIdentity].SendChain=c.Sessions[*partnerIdentity].RootChain.DeriveKey(CHAIN_LABEL)
	// c.Sessions[*partnerIdentity].ReceiveChain=c.Sessions[*partnerIdentity].RootChain.DeriveKey(CHAIN_LABEL)
	c.Sessions[*partnerIdentity].LastUpdate=c.Sessions[*partnerIdentity].SendCounter
	c.Sessions[*partnerIdentity].Change=false
	def=true
	}


	chain:=c.Sessions[*partnerIdentity].SendChain
	key:=chain.DeriveKey(KEY_LABEL)
	IV:=NewIV()


	c.Sessions[*partnerIdentity].SendChain=chain.DeriveKey(CHAIN_LABEL)


	message := &Message{
		Sender:   &c.Identity.PublicKey,
		Receiver: partnerIdentity,
		Counter: c.Sessions[*partnerIdentity].SendCounter,
		NextDHRatchet: &c.Sessions[*partnerIdentity].MyDHRatchet.PublicKey,
		LastUpdate: c.Sessions[*partnerIdentity].LastUpdate,
		Change: def,
	}

	data:=message.EncodeAdditionalData()
	message.Ciphertext=key.AuthenticatedEncrypt(plaintext,data,IV)
	message.IV=IV

	return message, nil
}

// ReceiveMessage is used to receive the given message and return the correct
// plaintext. This method is where most of the key derivation, ratcheting
// and out-of-order message handling logic happens.
func (c *Chatter) ReceiveMessage(message *Message) (string, error) {

	if _, exists := c.Sessions[*message.Sender]; !exists {
		return "", errors.New("Can't receive message from partner with no open session")
	}

	counterFail:=c.Sessions[*message.Sender].ReceiveCounter
	rootFail:=c.Sessions[*message.Sender].RootChain
	partnerFail:=c.Sessions[*message.Sender].PartnerDHRatchet
	receiveFail:=c.Sessions[*message.Sender].ReceiveChain


	if message.Counter>=c.Sessions[*message.Sender].ReceiveCounter{
		c.Sessions[*message.Sender].ReceiveCounter++
	}

	if message.Counter==c.Sessions[*message.Sender].ReceiveCounter && message.Change && !c.Sessions[*message.Sender].Change{
		root:=c.Sessions[*message.Sender].RootChain.DeriveKey(ROOT_LABEL)
		c.Sessions[*message.Sender].PartnerDHRatchet=message.NextDHRatchet
		c.Sessions[*message.Sender].RootChain=CombineKeys(root,DHCombine(message.NextDHRatchet, &c.Sessions[*message.Sender].MyDHRatchet.PrivateKey))
		c.Sessions[*message.Sender].ReceiveChain=c.Sessions[*message.Sender].RootChain.DeriveKey(CHAIN_LABEL)
		c.Sessions[*message.Sender].Change=true
	}

	val,err:=c.Sessions[*message.Sender].ReceiveChain.DeriveKey(KEY_LABEL).AuthenticatedDecrypt(message.Ciphertext,message.EncodeAdditionalData(),message.IV)
	
	if message.Counter==c.Sessions[*message.Sender].ReceiveCounter {
		if err!=nil{
			c.Sessions[*message.Sender].RootChain=rootFail
			c.Sessions[*message.Sender].PartnerDHRatchet=partnerFail
			c.Sessions[*message.Sender].ReceiveChain=receiveFail
			c.Sessions[*message.Sender].Change=false
			c.Sessions[*message.Sender].ReceiveCounter=counterFail
			return "",err
		}
		c.Sessions[*message.Sender].ReceiveChain=c.Sessions[*message.Sender].ReceiveChain.DeriveKey(CHAIN_LABEL)
		return val,err
	} else if message.Counter>c.Sessions[*message.Sender].ReceiveCounter {
		// early messages
		for i:=c.Sessions[*message.Sender].ReceiveCounter;i<message.Counter;i++ {
			if message.LastUpdate==i{
				root:=c.Sessions[*message.Sender].RootChain.DeriveKey(ROOT_LABEL)
				c.Sessions[*message.Sender].PartnerDHRatchet=message.NextDHRatchet
				c.Sessions[*message.Sender].RootChain=CombineKeys(root,DHCombine(message.NextDHRatchet, &c.Sessions[*message.Sender].MyDHRatchet.PrivateKey))
				c.Sessions[*message.Sender].ReceiveChain=c.Sessions[*message.Sender].RootChain.DeriveKey(CHAIN_LABEL)
				c.Sessions[*message.Sender].Change=true
			}
			c.Sessions[*message.Sender].CachedReceiveKeys[i]=c.Sessions[*message.Sender].ReceiveChain.DeriveKey(KEY_LABEL)
			c.Sessions[*message.Sender].ReceiveChain=c.Sessions[*message.Sender].ReceiveChain.DeriveKey(CHAIN_LABEL)
		}

		if message.LastUpdate==message.Counter{
			root:=c.Sessions[*message.Sender].RootChain.DeriveKey(ROOT_LABEL)
				c.Sessions[*message.Sender].PartnerDHRatchet=message.NextDHRatchet
				c.Sessions[*message.Sender].RootChain=CombineKeys(root,DHCombine(message.NextDHRatchet, &c.Sessions[*message.Sender].MyDHRatchet.PrivateKey))
				c.Sessions[*message.Sender].ReceiveChain=c.Sessions[*message.Sender].RootChain.DeriveKey(CHAIN_LABEL)
				c.Sessions[*message.Sender].Change=true
		}

		key:=c.Sessions[*message.Sender].ReceiveChain.DeriveKey(KEY_LABEL)
		data:=message.EncodeAdditionalData()
		val,err:=key.AuthenticatedDecrypt(message.Ciphertext,data,message.IV)
		c.Sessions[*message.Sender].ReceiveCounter=message.Counter
		c.Sessions[*message.Sender].ReceiveChain=c.Sessions[*message.Sender].ReceiveChain.DeriveKey(CHAIN_LABEL)
		if err!=nil{
			c.Sessions[*message.Sender].RootChain=rootFail
			c.Sessions[*message.Sender].PartnerDHRatchet=partnerFail
			c.Sessions[*message.Sender].ReceiveChain=receiveFail
			c.Sessions[*message.Sender].Change=false
			c.Sessions[*message.Sender].ReceiveCounter=counterFail
		}

		return val,err
		
	} else {
		// late messages
		key:=c.Sessions[*message.Sender].CachedReceiveKeys[message.Counter]
		data:=message.EncodeAdditionalData()
		val,err:=key.AuthenticatedDecrypt(message.Ciphertext,data,message.IV)
		if err==nil{
		key.Zeroize()
		}
		return val,err
	}
}

// EncodeAdditionalData encodes all of the non-ciphertext fields of a message
// into a single byte array, suitable for use as additional authenticated data
// in an AEAD scheme. You should not need to modify this code.
func (m *Message) EncodeAdditionalData() []byte {
	buf := make([]byte, 8+3*FINGERPRINT_LENGTH)

	binary.LittleEndian.PutUint32(buf, uint32(m.Counter))
	binary.LittleEndian.PutUint32(buf[4:], uint32(m.LastUpdate))

	if m.Sender != nil {
		copy(buf[8:], m.Sender.Fingerprint())
	}
	if m.Receiver != nil {
		copy(buf[8+FINGERPRINT_LENGTH:], m.Receiver.Fingerprint())
	}
	if m.NextDHRatchet != nil {
		copy(buf[8+2*FINGERPRINT_LENGTH:], m.NextDHRatchet.Fingerprint())
	}

	return buf
}

// NewChatter creates and initializes a new Chatter object. A long-term
// identity key is created and the map of sessions is initialized.
// You should not need to modify this code.
func NewChatter() *Chatter {
	c := new(Chatter)
	c.Identity = GenerateKeyPair()
	c.Sessions = make(map[PublicKey]*Session)
	return c
}

// EndSession erases all data for a session with the designated partner.
// All outstanding key material should be zeroized and the session erased.
func (c *Chatter) EndSession(partnerIdentity *PublicKey) error {

	if _, exists := c.Sessions[*partnerIdentity]; !exists {
		return errors.New("Don't have that session open to tear down")
	}

	delete(c.Sessions, *partnerIdentity)

	// TODO: your code here to zeroize remaining state

	return nil
}
