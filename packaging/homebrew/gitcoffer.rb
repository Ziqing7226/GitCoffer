class Gitcoffer < Formula
  desc "Encrypted git remote that lives on your own disk"
  homepage "https://github.com/Ziqing7226/GitCoffer"
  url "https://github.com/Ziqing7226/GitCoffer/archive/refs/tags/v1.0.0.tar.gz"
  sha256 "17dde536f214d5bdc907142a351064d8d69d8a01c502390c29b7e45e8f9f84b1"
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
