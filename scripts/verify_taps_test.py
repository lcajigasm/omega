#!/usr/bin/env python3
"""Parser checks for scripts/verify_taps.py. No network."""

import unittest

from verify_taps import FILES, TAP, TapError, check_once, normalize, versions_in


class VersionsTest(unittest.TestCase):
    def test_url_versions_agree(self):
        text = """
        url "https://github.com/lcajigasm/omega.git", branch: "main"
        url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.10.0_darwin_arm64.tar.gz"
        url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.10.0_linux_amd64.tar.gz"
        """
        self.assertEqual(versions_in(text), "0.10.0")

    def test_explicit_version_must_match_urls(self):
        text = """
        version "0.10.0"
        url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.10.0_darwin_arm64.tar.gz"
        """
        self.assertEqual(versions_in(text), "0.10.0")

    def test_mixed_versions_fail(self):
        text = """
        version "0.9.0"
        url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.10.0_darwin_arm64.tar.gz"
        """
        with self.assertRaises(TapError):
            versions_in(text)

    def test_archive_name_must_match_url(self):
        text = 'url "https://github.com/lcajigasm/omega/releases/download/v0.10.0/omega_0.9.0_linux_amd64.tar.gz"'
        with self.assertRaises(TapError):
            versions_in(text)

    def test_head_url_alone_is_not_a_version(self):
        text = 'url "https://github.com/lcajigasm/omega.git", branch: "main"'
        with self.assertRaises(TapError):
            versions_in(text)

    def test_normalize_tag(self):
        self.assertEqual(normalize("v0.10.0"), "0.10.0")
        self.assertEqual(normalize(" 0.10.0 "), "0.10.0")

    def test_only_homebrew_tap(self):
        self.assertEqual(TAP, "lcajigasm/homebrew-tap")
        self.assertNotIn("homebrew-omega", TAP)

    def test_cask_version(self):
        text = (
            'cask "omega-app" do\n  version "0.17.0"\n  sha256 "abc"\n'
            '  url "https://github.com/lcajigasm/omega/releases/download/'
            'v#{version}/omega-macos-universal.dmg"\nend\n'
        )
        self.assertEqual(versions_in(text), "0.17.0")

    def test_formula_and_cask_are_both_checked(self):
        # The tap that has the new formula and no cask, or an old one, is
        # the tap where brew install --cask omega-app does not work.
        def fetch(repo, path):
            if path == "Casks/omega-app.rb":
                raise TapError("HTTP 404")
            return 'version "0.17.0"\n'

        self.assertEqual(FILES, ("Formula/omega.rb", "Casks/omega-app.rb"))
        problems = check_once("0.17.0", fetch)
        self.assertEqual(len(problems), 1)
        self.assertIn("Casks/omega-app.rb", problems[0])
        self.assertEqual(check_once("0.17.0", lambda repo, path: 'version "0.17.0"\n'), [])


if __name__ == "__main__":
    unittest.main()
