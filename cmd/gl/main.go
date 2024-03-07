package main

import (
	"fmt"
	"log"
	"runtime"

	"github.com/go-gl/gl/v4.6-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
)

const (
	vertexShaderSource = `
  #version 330 core
  in vec2 position;
  uniform mat4 projection;
  void main() {
    gl_Position = projection * vec4(position, 0.0, 1.0);
  }`
	fragmentShaderSource = `
	#version 330 core
	out vec4 color;
	void main() {
		color = vec4(1.0, 0.0, 0.0, 1.0);
	}`
)

func init() {
	// This is needed to arrange that main() runs on main thread.
	// See documentation for functions that are only allowed to be called from the main thread.
	runtime.LockOSThread()
}

func main() {
	err := glfw.Init()
	if err != nil {
		log.Fatalln("Unable to initialize glfw", err)
	}
	defer glfw.Terminate()

	window, err := glfw.CreateWindow(640, 480, "Testing", nil, nil)
	if err != nil {
		panic(err)
	}

	err = gl.Init()
	if err != nil {
		panic(err)
	}

	version := gl.GoStr(gl.GetString(gl.VERSION))
	log.Println("OpenGL version", version)

	window.MakeContextCurrent()

	// Initialize OpenGL shaders and program
	program, err := newProgram(vertexShaderSource, fragmentShaderSource)
	if err != nil {
		panic(err)
	}
	gl.UseProgram(program)
	defer gl.DeleteProgram(program)

	vao, _ := makeSquare()

	// Set up the projection matrix
	projection := mgl32.Ortho2D(-1, 1, -1, 1)
	projectionUniform := gl.GetUniformLocation(program, gl.Str("projection\x00"))
	gl.UniformMatrix4fv(projectionUniform, 1, false, &projection[0])

	for !window.ShouldClose() {
		doOpenGL(vao)
		window.SwapBuffers()
		glfw.PollEvents()
	}
}

func doOpenGL(vao uint32) {
	gl.Clear(gl.COLOR_BUFFER_BIT)

	// Draw the square
	gl.BindVertexArray(vao)
	gl.DrawArrays(gl.TRIANGLE_FAN, 0, 4)
	gl.BindVertexArray(0)

	gl.Flush()
}

func newProgram(vertexShaderSource, fragmentShaderSource string) (uint32, error) {
	vertexShader, err := compileShader(vertexShaderSource, gl.VERTEX_SHADER)
	if err != nil {
		return 0, err
	}

	fragmentShader, err := compileShader(fragmentShaderSource, gl.FRAGMENT_SHADER)
	if err != nil {
		return 0, err
	}

	program := gl.CreateProgram()
	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)

		logMessage := make([]byte, logLength)
		gl.GetProgramInfoLog(program, logLength, nil, &logMessage[0])

		return 0, fmt.Errorf("program link error: %v", string(logMessage))
	}

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)

	return program, nil
}

func compileShader(source string, shaderType uint32) (uint32, error) {
	shader := gl.CreateShader(shaderType)

	cSource, free := gl.Strs(source + "\x00")
	gl.ShaderSource(shader, 1, cSource, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)

		logMessage := make([]byte, logLength)
		gl.GetShaderInfoLog(shader, logLength, nil, &logMessage[0])

		return 0, fmt.Errorf("shader compile error: %v", string(logMessage))
	}

	return shader, nil
}

func makeSquare() (uint32, uint32) {
	vertices := []float32{
		-0.5, 0.5,
		0.5, 0.5,
		0.5, -0.5,
		-0.5, -0.5,
	}

	var vao, vbo uint32
	gl.GenVertexArrays(1, &vao)
	gl.GenBuffers(1, &vbo)

	gl.BindVertexArray(vao)

	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, 4*len(vertices), gl.Ptr(vertices), gl.STATIC_DRAW)

	// Position attribute
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 2*4, gl.PtrOffset(0))
	gl.EnableVertexAttribArray(0)

	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	gl.BindVertexArray(0)

	return vao, vbo
}
