class Centrifugo < Formula
  desc "Centrifugo"
  homepage "https://github.com/centrifugal/centrifugo"
  url "https://github.com/centrifugal/centrifugo/releases/download/v1.7.1/centrifugo-1.7.1-darwin-amd64.zip"
  sha256 "4a19664add41a343af0cd911b4392765f52273a0d3025b42bafd852bc81e4b17"

  def install
    system "chmod", "+x", "centrifugo"
    system "mkdir", "#{prefix}/bin"
    system "cp", "centrifugo", "#{prefix}/bin/centrifugo"
  end
end
