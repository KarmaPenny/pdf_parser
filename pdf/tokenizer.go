package pdf

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
)

var whitespace = []byte("\x00\t\n\f\r ")
var delimiters = []byte("()<>[]/%")

type Tokenizer struct {
	*bufio.Reader
}

func NewTokenizer(reader io.Reader) *Tokenizer {
	return &Tokenizer{bufio.NewReader(reader)}
}

func (tokenizer *Tokenizer) NextToken() (*Token, error) {
	// skip leading whitespace
	b, err := tokenizer.SkipWhitespace()
	if err != nil {
		return nil, err
	}

	// start a new token
	token := NewToken(b)

	// if start or end of array then return as token
	if b == '[' || b == ']' {
		return token, nil
	}

	// if start of string
	if b == '(' {
		// find balanced closing bracket
		for open_parens := 1; open_parens > 0; {
			// read next byte
			b, err = tokenizer.ReadByte()
			if err != nil {
				token.WriteByte(')')
				return token, nil
			}

			// if this is the start of an escape sequence
			if b == '\\' {
				// read next byte
				b, err = tokenizer.ReadByte()
				if err != nil {
					token.WriteByte('\\')
					token.WriteByte(')')
					return token, nil
				}

				// ignore escaped line breaks \n or \r or \r\n
				if b == '\n' {
					continue
				}
				if b == '\r' {
					// read next byte
					b, err = tokenizer.ReadByte()
					if err != nil {
						token.WriteByte(')')
						return token, nil
					}
					// if byte is not a new line then unread it
					if b != '\n' {
						tokenizer.UnreadByte()
					}
					continue
				}

				// special escape values
				if b == 'n' {
					b = '\n'
				} else if b == 'r' {
					b = '\r'
				} else if b == 't' {
					b = '\t'
				} else if b == 'b' {
					b = '\b'
				} else if b == 'f' {
					b = '\f'
				}

				// if this is the start of an octal character code
				if b >= '0' && b <= '7' {
					// add byte to character code
					code := bytes.NewBuffer([]byte{b})

					// add at most 2 more bytes to code
					for i := 0; i < 2; i++ {
						// read next byte
						b, err = tokenizer.ReadByte()
						if err != nil {
							break
						}

						// if next byte is not part of the octal code
						if b < '0' || b > '7' {
							// unread the byte and stop collecting code
							tokenizer.UnreadByte()
							break
						}

						// add byte to code
						code.WriteByte(b)
					}

					// convert code into byte
					val, err := strconv.ParseUint(string(code.Bytes()), 8, 8)
					if err != nil {
						// octal code is too large so ignore last byte
						tokenizer.UnreadByte()
						val, _ = strconv.ParseUint(string(code.Bytes()[:code.Len()-1]), 8, 8)
					}
					b = byte(val)
				}

				// add byte to token and continue
				token.WriteByte(b)
				continue
			}

			// add byte to token
			token.WriteByte(b)

			// keep track of number of open parens
			if b == '(' {
				open_parens++
			} else if b == ')' {
				open_parens--
			}
		}

		// return string
		return token, nil
	}

	// if start of name
	if b == '/' {
		// parse name
		for {
			// read in the next byte
			b, err = tokenizer.ReadByte()
			if err != nil {
				return token, nil
			}

			// if the next byte is whitespace or delimiter then unread it and return the token
			if bytes.IndexByte(delimiters, b) >= 0 || bytes.IndexByte(whitespace, b) >= 0 {
				tokenizer.UnreadByte()
				return token, nil
			}

			// if next byte is the start of a hex character code
			if b == '#' {
				// read in the hex code
				code := []byte{'0', '0'}
				for i := 0; i < 2; i++ {
					b, err = tokenizer.ReadByte()
					if err != nil {
						break
					}
					if !IsHex(b) {
						tokenizer.UnreadByte()
						break
					}
					code[i] = b
				}

				// convert the hex code to a byte
				val, _ := strconv.ParseUint(string(code), 16, 8)
				b = byte(val)
			}

			// add byte to token
			token.WriteByte(b)
		}
	}

	// if start of hex string or dictionary
	if b == '<' {
		// get next byte
		b, err = tokenizer.ReadByte()
		if err != nil {
			token.WriteByte('>')
			return token, nil
		}

		// if this is the dictionary start marker then return token
		if b == '<' {
			token.WriteByte(b)
			return token, nil
		} else {
			tokenizer.UnreadByte()
		}

		// read hex code pairs until end of hex string or file
		for {
			code := []byte{'0', '0'}
			for i := 0; i < 2; {
				b, err = tokenizer.SkipWhitespace()
				if err != nil || b == '>' {
					if i > 0 {
						val, _ := strconv.ParseUint(string(code), 16, 8)
						token.WriteByte(byte(val))
					}
					token.WriteByte('>')
					return token, nil
				}
				if !IsHex(b) {
					continue
				}
				code[i] = b
				i++
			}
			val, _ := strconv.ParseUint(string(code), 16, 8)
			token.WriteByte(byte(val))
		}
	}

	// if end of dictionary
	if b == '>' {
		b, err = tokenizer.ReadByte()
		if err == nil  && b != '>' {
			tokenizer.UnreadByte()
		}
		token.WriteByte('>')
		return token, nil
	}

	// set token is number if first byte is a digit
	token.IsNumber = b >= '0' && b <= '9'

	// ordinary token, scan until next whitespace or delimiter
	for {
		// get next byte
		b, err = tokenizer.ReadByte()
		if err != nil {
			return token, nil
		}

		// if byte is whitespace or delimiter then unread byte and return token
		if bytes.IndexByte(whitespace, b) >= 0 || bytes.IndexByte(delimiters, b) >= 0 {
			tokenizer.UnreadByte()
			return token, nil
		}

		// update is number
		token.IsNumber = token.IsNumber && b >= '0' && b <= '9'

		// add byte to token
		token.WriteByte(b)
	}
}

func (tokenizer *Tokenizer) SkipWhitespace() (byte, error) {
	for {
		// get next byte
		b, err := tokenizer.ReadByte()
		if err != nil {
			return 0, err
		}

		// advance if next byte is whitespace
		if bytes.IndexByte(whitespace, b) >= 0 {
			continue
		}

		// if next byte is start of a comment then advance until next line
		if b == '%' {
			_, err = tokenizer.ReadBytes('\n')
			if err != nil {
				return 0, err
			}
			continue
		}

		// next byte is neither comment or whitespace so return
		return b, nil
	}
}
