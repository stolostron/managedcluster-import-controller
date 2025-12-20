package yaml_test

import (
	"bytes"

	yaml "github.com/oasdiff/yaml3"
	. "gopkg.in/check.v1"
)

func (s *S) TestOrigin_Disabled(c *C) {
	input := `
root:
    hello: world
`

	dec := yaml.NewDecoder(bytes.NewBufferString(input[1:]))
	dec.Origin(false)
	var out any
	err := dec.Decode(&out)
	c.Assert(err, IsNil)
	result, err := yaml.Marshal(out)
	c.Assert(err, IsNil)

	buf := new(bytes.Buffer)
	buf.Write(result)

	c.Assert(buf.String(), Equals, input[1:])
}

func (s *S) TestOrigin_Map(c *C) {
	input := `
root:
    hello: world
    object:
        foo: bar
`

	dec := yaml.NewDecoder(bytes.NewBufferString(input[1:]))
	dec.Origin(true)
	var out any
	err := dec.Decode(&out)
	c.Assert(err, IsNil)
	result, err := yaml.Marshal(out)
	c.Assert(err, IsNil)

	buf := new(bytes.Buffer)
	buf.Write(result)

	output := `
root:
    __origin__:
        fields:
            hello:
                column: 5
                line: 2
                name: hello
        key:
            column: 1
            line: 1
            name: root
    hello: world
    object:
        __origin__:
            fields:
                foo:
                    column: 9
                    line: 4
                    name: foo
            key:
                column: 5
                line: 3
                name: object
        foo: bar
`

	c.Assert(buf.String(), Equals, output[1:])
}

func (s *S) TestOrigin_SequenceOfMaps(c *C) {
	input := `
root:
    continents:
        - name: europe
          size: 10
        - name: america
          size: 20
`

	dec := yaml.NewDecoder(bytes.NewBufferString(input[1:]))
	dec.Origin(true)
	var out any
	err := dec.Decode(&out)
	c.Assert(err, IsNil)
	result, err := yaml.Marshal(out)
	c.Assert(err, IsNil)

	buf := new(bytes.Buffer)
	buf.Write(result)

	output := `
root:
    __origin__:
        key:
            column: 1
            line: 1
            name: root
    continents:
        - __origin__:
            fields:
                name:
                    column: 11
                    line: 3
                    name: name
                size:
                    column: 11
                    line: 4
                    name: size
            key:
                column: 11
                line: 3
                name: name
          name: europe
          size: 10
        - __origin__:
            fields:
                name:
                    column: 11
                    line: 5
                    name: name
                size:
                    column: 11
                    line: 6
                    name: size
            key:
                column: 11
                line: 5
                name: name
          name: america
          size: 20
`

	c.Assert(buf.String(), Equals, output[1:])
}

func (s *S) TestOrigin_DuplicateKey(c *C) {
	input := `
root:
    __origin__: test
`

	dec := yaml.NewDecoder(bytes.NewBufferString(input[1:]))
	dec.Origin(true)
	var out any
	err := dec.Decode(&out)
	c.Assert(err, ErrorMatches, "yaml: unmarshal errors:\n  line 0: mapping key \"__origin__\" already defined at line 2")
}
