package core

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dagger/testctx"
	"github.com/moby/buildkit/identity"
	"github.com/stretchr/testify/require"

	"dagger.io/dagger"
)

type HostSuite struct{}

func TestHost(t *testing.T) {
	testctx.New(t, Middleware()...).RunTests(HostSuite{})
}

func (HostSuite) TestWorkdir(ctx context.Context, t *testctx.T) {
	t.Run("contains the workdir's content", func(ctx context.Context, t *testctx.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "foo"), []byte("bar"), 0o600)
		require.NoError(t, err)

		c := connect(ctx, t, dagger.WithWorkdir(dir))

		contents, err := c.Container().
			From(alpineImage).
			WithMountedDirectory("/host", c.Host().Directory(".")).
			WithExec([]string{"ls", "/host"}).
			Stdout(ctx)
		require.NoError(t, err)
		require.Equal(t, "foo\n", contents)
	})

	t.Run("does NOT re-sync on each call", func(ctx context.Context, t *testctx.T) {
		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "foo"), []byte("bar"), 0o600)
		require.NoError(t, err)

		c := connect(ctx, t, dagger.WithWorkdir(dir))

		contents, err := c.Container().
			From(alpineImage).
			WithMountedDirectory("/host", c.Host().Directory(".")).
			WithExec([]string{"ls", "/host"}).
			Stdout(ctx)
		require.NoError(t, err)
		require.Equal(t, "foo\n", contents)

		err = os.WriteFile(filepath.Join(dir, "fizz"), []byte("buzz"), 0o600)
		require.NoError(t, err)

		contents, err = c.Container().
			From(alpineImage).
			WithMountedDirectory("/host", c.Host().Directory(".")).
			WithExec([]string{"ls", "/host"}).
			Stdout(ctx)
		require.NoError(t, err)
		require.Equal(t, "foo\n", contents)
	})
}

func (HostSuite) TestWorkdirExcludeInclude(ctx context.Context, t *testctx.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("2"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt.rar"), []byte("3"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir", "sub-file"), []byte("goodbye"), 0o600))

	c := connect(ctx, t, dagger.WithWorkdir(dir))

	t.Run("exclude", func(ctx context.Context, t *testctx.T) {
		wd := c.Host().Directory(".", dagger.HostDirectoryOpts{
			Exclude: []string{"*.rar"},
		})

		contents, err := c.Container().
			From(alpineImage).
			WithMountedDirectory("/host", wd).
			WithExec([]string{"ls", "/host"}).
			Stdout(ctx)
		require.NoError(t, err)
		require.Equal(t, "a.txt\nb.txt\nsubdir\n", contents)
	})

	t.Run("exclude directory", func(ctx context.Context, t *testctx.T) {
		wd := c.Host().Directory(".", dagger.HostDirectoryOpts{
			Exclude: []string{"subdir"},
		})

		contents, err := c.Container().
			From(alpineImage).
			WithMountedDirectory("/host", wd).
			WithExec([]string{"ls", "/host"}).
			Stdout(ctx)
		require.NoError(t, err)
		require.Equal(t, "a.txt\nb.txt\nc.txt.rar\n", contents)
	})

	t.Run("include", func(ctx context.Context, t *testctx.T) {
		wd := c.Host().Directory(".", dagger.HostDirectoryOpts{
			Include: []string{"*.rar"},
		})

		contents, err := c.Container().
			From(alpineImage).
			WithMountedDirectory("/host", wd).
			WithExec([]string{"ls", "/host"}).
			Stdout(ctx)
		require.NoError(t, err)
		require.Equal(t, "c.txt.rar\n", contents)
	})

	t.Run("exclude overrides include", func(ctx context.Context, t *testctx.T) {
		wd := c.Host().Directory(".", dagger.HostDirectoryOpts{
			Include: []string{"*.txt"},
			Exclude: []string{"b.txt"},
		})

		contents, err := c.Container().
			From(alpineImage).
			WithMountedDirectory("/host", wd).
			WithExec([]string{"ls", "/host"}).
			Stdout(ctx)
		require.NoError(t, err)
		require.Equal(t, "a.txt\n", contents)
	})

	t.Run("include does not override exclude", func(ctx context.Context, t *testctx.T) {
		wd := c.Host().Directory(".", dagger.HostDirectoryOpts{
			Include: []string{"a.txt"},
			Exclude: []string{"*.txt"},
		})

		contents, err := c.Container().
			From(alpineImage).
			WithMountedDirectory("/host", wd).
			WithExec([]string{"ls", "/host"}).
			Stdout(ctx)
		require.NoError(t, err)
		require.Equal(t, "", contents)
	})
}

func (HostSuite) TestDirectoryRelative(ctx context.Context, t *testctx.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "some-file"), []byte("hello"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "some-dir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "some-dir", "sub-file"), []byte("goodbye"), 0o600))

	c := connect(ctx, t, dagger.WithWorkdir(dir))

	t.Run(". is same as workdir", func(ctx context.Context, t *testctx.T) {
		wdID1, err := c.Host().Directory(".").ID(ctx)
		require.NoError(t, err)

		wdID2, err := c.Host().Directory(".").ID(ctx)
		require.NoError(t, err)

		require.Equal(t, wdID1, wdID2)
	})

	t.Run("./foo is relative to workdir", func(ctx context.Context, t *testctx.T) {
		contents, err := c.Host().Directory("some-dir").Entries(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"sub-file"}, contents)
	})

	t.Run("../ does not allow escaping", func(ctx context.Context, t *testctx.T) {
		_, err := c.Host().Directory("../").ID(ctx)
		require.Error(t, err)

		// don't reveal the workdir location
		require.NotContains(t, err.Error(), dir)
	})
}

func (HostSuite) TestDirectoryAbsolute(ctx context.Context, t *testctx.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "some-file"), []byte("hello"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "some-dir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "some-dir", "sub-file"), []byte("goodbye"), 0o600))

	c := connect(ctx, t, dagger.WithWorkdir(dir))

	entries, err := c.Host().Directory(filepath.Join(dir, "some-dir")).Entries(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"sub-file"}, entries)
}

func (HostSuite) TestDirectoryHome(ctx context.Context, t *testctx.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	subdir := filepath.Join(".cache", "dagger-test-"+identity.NewID())

	require.NoError(t, os.MkdirAll(filepath.Join(home, subdir), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, subdir, "some-file"), []byte("hello"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(home, subdir, "some-dir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, subdir, "some-dir", "sub-file"), []byte("goodbye"), 0o600))

	c := connect(ctx, t, dagger.WithWorkdir("/tmp"))

	entries, err := c.Host().Directory(filepath.Join("~", subdir, "some-dir")).Entries(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"sub-file"}, entries)
}

func (HostSuite) TestSetSecretFile(ctx context.Context, t *testctx.T) {
	// Generate 512000 random bytes (non UTF-8)
	// This is our current limit: secrets break at 512001 bytes
	data := make([]byte, 512000)
	_, err := rand.Read(data)
	if err != nil {
		panic(err)
	}

	// Compute the MD5 hash of the data
	hash := md5.Sum(data)
	hashStr := hex.EncodeToString(hash[:])

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "some-file"), data, 0o600))

	c := connect(ctx, t, dagger.WithWorkdir(dir))

	t.Run("non utf8 binary data is properly set as secret", func(ctx context.Context, t *testctx.T) {
		//nolint:staticcheck // SA1019 deprecated
		secret := c.Host().SetSecretFile("mysecret", filepath.Join(dir, "some-file"))

		output, err := c.Container().From(alpineImage).
			WithEnvVariable("CACHEBUST", identity.NewID()).
			WithMountedSecret("/mysecret", secret).
			WithExec([]string{"md5sum", "/mysecret"}).
			Stdout(ctx)

		require.NoError(t, err)

		// Extract the MD5 hash from the command output
		hashStrCmd := strings.Split(output, " ")[0]

		require.Equal(t, hashStr, hashStrCmd)
	})
}

func (HostSuite) TestDirectoryExcludeInclude(ctx context.Context, t *testctx.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("2"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt.rar"), []byte("3"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir", "d.txt"), []byte("1"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir", "e.txt"), []byte("2"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir", "f.txt.rar"), []byte("3"), 0o600))

	c := connect(ctx, t)

	t.Run("exclude", func(ctx context.Context, t *testctx.T) {
		entries, err := c.Host().Directory(dir, dagger.HostDirectoryOpts{
			Exclude: []string{"*.rar"},
		}).Entries(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"a.txt", "b.txt", "subdir/"}, entries)
	})

	t.Run("include", func(ctx context.Context, t *testctx.T) {
		entries, err := c.Host().Directory(dir, dagger.HostDirectoryOpts{
			Include: []string{"*.rar"},
		}).Entries(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"c.txt.rar"}, entries)
	})

	t.Run("exclude overrides include", func(ctx context.Context, t *testctx.T) {
		entries, err := c.Host().Directory(dir, dagger.HostDirectoryOpts{
			Include: []string{"*.txt"},
			Exclude: []string{"b.txt"},
		}).Entries(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"a.txt"}, entries)
	})

	t.Run("include does not override exclude", func(ctx context.Context, t *testctx.T) {
		entries, err := c.Host().Directory(dir, dagger.HostDirectoryOpts{
			Include: []string{"a.txt"},
			Exclude: []string{"*.txt"},
		}).Entries(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{}, entries)
	})
}

func (HostSuite) TestDirectoryCacheBehavior(ctx context.Context, t *testctx.T) {
	baseDir := t.TempDir()
	c := connect(ctx, t)

	tests := []struct {
		name            string
		opts            dagger.HostDirectoryOpts
		expectedEntries []string
		expectedContent string
	}{
		{
			name:            "default aka cache",
			opts:            dagger.HostDirectoryOpts{},
			expectedEntries: []string{"file1.txt"},
			expectedContent: "1",
		},
		{
			name:            "explicit cache",
			opts:            dagger.HostDirectoryOpts{NoCache: false},
			expectedEntries: []string{"file1.txt"},
			expectedContent: "1",
		},
		{
			name:            "explicit no cache",
			opts:            dagger.HostDirectoryOpts{NoCache: true},
			expectedEntries: []string{"file1.txt", "file2.txt"},
			expectedContent: "12",
		},
	}

	for _, test := range tests {
		setup := func() (string, *dagger.Directory) {
			dir := filepath.Join(baseDir, identity.NewID())
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("1"), 0o600))

			directory := c.Host().Directory(dir, test.opts)
			return dir, directory
		}

		t.Run(test.name, func(ctx context.Context, t *testctx.T) {
			dir, directory := setup()
			entries, err := directory.Entries(ctx)
			require.NoError(t, err)
			require.Equal(t, []string{"file1.txt"}, entries)

			require.NoError(t, os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("1"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("12"), 0o600))

			entries, err = directory.Entries(ctx)
			require.NoError(t, err)
			require.Equal(t, test.expectedEntries, entries)

			contents, err := directory.File("file1.txt").Contents(ctx)
			require.NoError(t, err)
			require.Equal(t, test.expectedContent, contents)
		})

		t.Run(test.name+" fresh query", func(ctx context.Context, t *testctx.T) {
			dir, directory := setup()
			entries, err := directory.Entries(ctx)
			require.NoError(t, err)
			require.Equal(t, []string{"file1.txt"}, entries)

			require.NoError(t, os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("1"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("12"), 0o600))

			directory = c.Host().Directory(dir, test.opts)
			entries, err = directory.Entries(ctx)
			require.NoError(t, err)
			require.Equal(t, test.expectedEntries, entries)

			contents, err := directory.File("file1.txt").Contents(ctx)
			require.NoError(t, err)
			require.Equal(t, test.expectedContent, contents)
		})

		t.Run(test.name+" explicit sync always caches", func(ctx context.Context, t *testctx.T) {
			dir, directory := setup()
			directory, err := directory.Sync(ctx)
			require.NoError(t, err)

			entries, err := directory.Entries(ctx)
			require.NoError(t, err)
			require.Equal(t, []string{"file1.txt"}, entries)

			require.NoError(t, os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("1"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("12"), 0o600))

			entries, err = directory.Entries(ctx)
			require.NoError(t, err)
			// note the expectation here doesn't vary with test.expected
			require.Equal(t, []string{"file1.txt"}, entries)

			contents, err := directory.File("file1.txt").Contents(ctx)
			require.NoError(t, err)
			// note the expectation here doesn't vary with test.expected
			require.Equal(t, "1", contents)
		})
	}
}

func (HostSuite) TestFile(ctx context.Context, t *testctx.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("1"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "subdir", "d.txt"), []byte("hello world"), 0o600))

	c := connect(ctx, t)

	t.Run("get simple file", func(ctx context.Context, t *testctx.T) {
		content, err := c.Host().File(filepath.Join(dir, "a.txt")).Contents(ctx)

		require.NoError(t, err)
		require.Equal(t, "1", content)
	})

	t.Run("get nested file", func(ctx context.Context, t *testctx.T) {
		content, err := c.Host().File(filepath.Join(dir, "subdir", "d.txt")).Contents(ctx)

		require.NoError(t, err)
		require.Equal(t, "hello world", content)
	})
}

func (HostSuite) TestFileCacheBehavior(ctx context.Context, t *testctx.T) {
	dir := t.TempDir()
	c := connect(ctx, t)

	tests := []struct {
		name     string
		opts     []dagger.HostFileOpts
		expected string
	}{
		{
			name:     "default aka cache",
			opts:     []dagger.HostFileOpts{},
			expected: "1",
		},
		{
			name:     "explicit cache",
			opts:     []dagger.HostFileOpts{{NoCache: false}},
			expected: "1",
		},
		{
			name:     "explicit no cache",
			opts:     []dagger.HostFileOpts{{NoCache: true}},
			expected: "12",
		},
	}

	for _, test := range tests {
		setup := func() (string, *dagger.File) {
			bPath := filepath.Join(dir, rand.Text())
			require.NoError(t, os.WriteFile(bPath, []byte("1"), 0o600))

			file := c.Host().File(bPath, test.opts...)
			return bPath, file
		}

		t.Run(test.name, func(ctx context.Context, t *testctx.T) {
			bPath, file := setup()
			content, err := file.Contents(ctx)
			require.NoError(t, err)
			require.Equal(t, "1", content)

			require.NoError(t, os.WriteFile(bPath, []byte("12"), 0o600))

			content, err = file.Contents(ctx)
			require.NoError(t, err)
			require.Equal(t, test.expected, content)
		})

		t.Run(test.name+" fresh query", func(ctx context.Context, t *testctx.T) {
			bPath, file := setup()
			content, err := file.Contents(ctx)
			require.NoError(t, err)
			require.Equal(t, "1", content)

			require.NoError(t, os.WriteFile(bPath, []byte("12"), 0o600))

			file = c.Host().File(bPath, test.opts...)
			content, err = file.Contents(ctx)
			require.NoError(t, err)
			require.Equal(t, test.expected, content)
		})

		t.Run(test.name+" explicit sync always caches", func(ctx context.Context, t *testctx.T) {
			bPath, file := setup()
			file, err := file.Sync(ctx)
			require.NoError(t, err)

			content, err := file.Contents(ctx)
			require.NoError(t, err)
			require.Equal(t, "1", content)

			require.NoError(t, os.WriteFile(bPath, []byte("12"), 0o600))

			content, err = file.Contents(ctx)
			require.NoError(t, err)
			// note the expectation here doesn't vary with test.expected
			require.Equal(t, "1", content)
		})
	}
}
