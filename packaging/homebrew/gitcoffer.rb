class Gitcoffer < Formula
  desc "Encrypted git remote that lives on your own disk"
  homepage "https://github.com/Ziqing7226/GitCoffer"
  url "https://github.com/Ziqing7226/GitCoffer/archive/refs/tags/v1.0.0.tar.gz"
  # sha256 of the v1.0.0 source archive — filled from the tagged release
  # at submission time (the placeholder is intentionally invalid).
  sha256 "fill-in-when-v1.0.0-is-tagged"
  license "MIT"

  livecheck do
    url :stable
    strategy :github_latest
  end

  depends_on "go" => :build
  depends_on "git"

  def install
    ldflags = "-s -w -X main.version=v#{version}"
    system "go", "build", "-trimpath", "-ldflags", ldflags, "-o", bin/"gitcoffer", "./cmd/gitcoffer"
    system "go", "build", "-trimpath", "-ldflags", ldflags, "-o", bin/"git-remote-coffer", "./cmd/git-remote-coffer"
  end

  test do
    assert_match "gitcoffer", shell_output("#{bin}/gitcoffer version")
    output = shell_output("git ls-remote coffer::#{testpath}/nope.coffer 2>&1", 128)
    assert_match "not a coffer vault", output
  end
end
