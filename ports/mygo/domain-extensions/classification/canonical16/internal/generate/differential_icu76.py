#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""固定 ICU76/Unicode16 独立差分；不是 Swift oracle。所有输入均为合成测试资料。"""
import argparse
import ctypes as C
import hashlib
import json
import platform
import random
import struct
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SEED = 0x16C0FFEE

def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def records(path):
    for line in path.read_text(encoding='utf-8').splitlines():
        body = line.split('#', 1)[0].strip()
        if body and not body.startswith('@'):
            yield [part.strip() for part in body.split(';')]

def decode_hex(text):
    return ''.join(chr(int(x, 16)) for x in text.split())

class ICU:
    def __init__(self):
        self.uc = C.CDLL('libicuuc.so.76')
        self.i18n = C.CDLL('libicui18n.so.76')
        def bind(lib, name, args, result):
            fn = getattr(lib, name + '_76')
            fn.argtypes, fn.restype = args, result
            return fn
        self.version = bind(self.uc, 'u_getUnicodeVersion', [C.POINTER(C.c_uint8)], None)
        version = (C.c_uint8*4)()
        self.version(version)
        self.unicode_version = '.'.join(map(str, version))
        if self.unicode_version != '16.0.0.0':
            raise RuntimeError('ICU is not Unicode16: ' + self.unicode_version)
        get_version = bind(self.uc, 'u_getVersion', [C.POINTER(C.c_uint8)], None)
        get_version(version)
        self.icu_version = '.'.join(map(str, version))
        self.normalizers = {}
        for kind in ['NFC', 'NFKCCasefold']:
            get = bind(self.uc, 'unorm2_get'+kind+'Instance', [C.POINTER(C.c_int)], C.c_void_p)
            error = C.c_int(0)
            self.normalizers[kind] = get(C.byref(error))
            if error.value > 0:
                raise RuntimeError('normalizer error: ' + str(error.value))
        self.normalize = bind(self.uc, 'unorm2_normalize', [C.c_void_p, C.POINTER(C.c_uint16), C.c_int, C.POINTER(C.c_uint16), C.c_int, C.POINTER(C.c_int)], C.c_int)
        self.whitespace = bind(self.uc, 'u_isUWhiteSpace', [C.c_int], C.c_int8)
        self.open = bind(self.i18n, 'ubrk_open', [C.c_int, C.c_char_p, C.POINTER(C.c_uint16), C.c_int, C.POINTER(C.c_int)], C.c_void_p)
        self.first = bind(self.i18n, 'ubrk_first', [C.c_void_p], C.c_int)
        self.next = bind(self.i18n, 'ubrk_next', [C.c_void_p], C.c_int)
        self.close = bind(self.i18n, 'ubrk_close', [C.c_void_p], None)
    def utf16(self, text):
        data = text.encode('utf-16-le')
        return data, (C.c_uint16*(len(data)//2)).from_buffer_copy(data)
    def norm(self, kind, text):
        _, source = self.utf16(text)
        output = (C.c_uint16*max(32, len(source)*4))()
        error = C.c_int(0)
        count = self.normalize(self.normalizers[kind], source, len(source), output, len(output), C.byref(error))
        if error.value == 15:
            output = (C.c_uint16*(count+1))()
            error.value = 0
            count = self.normalize(self.normalizers[kind], source, len(source), output, len(output), C.byref(error))
        if error.value > 0:
            raise RuntimeError('ICU normalize error: ' + str(error.value))
        return bytes(output)[:count*2].decode('utf-16-le')
    def display(self, text):
        data, source = self.utf16(text)
        error = C.c_int(0)
        iterator = self.open(0, b'root', source, len(source), C.byref(error))
        if error.value > 0 or not iterator:
            raise RuntimeError('ICU grapheme error: ' + str(error.value))
        parts, word = [], []
        try:
            start = self.first(iterator)
            while True:
                end = self.next(iterator)
                if end == -1:
                    break
                cluster = data[2*start:2*end].decode('utf-16-le')
                if self.whitespace(ord(cluster[0])):
                    if word:
                        parts.append(''.join(word))
                        word = []
                else:
                    word.append(cluster)
                start = end
            if word:
                parts.append(''.join(word))
            return ' '.join(parts)
        finally:
            self.close(iterator)

def corpus(ucd):
    rng = random.Random(SEED)
    cases = ['', '\x00', '\U0010ffff', '\u00a8', ' \u0301X', 'A \u0301B',
             'a\n\u0301b', '\u0600 X', 'A\r\nB', 'A\u034f\u030a',
             'A\u0315\u034f\u0300', '\u1100\u200d\u1161\u11a8',
             'Straße', 'İIı', 'Σςσ', 'ＥﬃÉ', '\u0915\u094d\u0915',
             '\u0915\u093c\u094d\u200d\u0915', '👩\u0301\u200d👩',
             '🇸🇬🇬🇧🇺', '\u034f', '\u200b', '\u00ad\u200d\ufe0f']
    for n in [29, 30, 31, 32, 60, 61, 4096, 65536]:
        cases.extend(['a'+'\u0315'*n+'\u0300'*n, '\u0315'*n+'\u0300'*n,
                      'a'+'\u0305'*n+'\u0301', 'A'+'\u0315'*n+'\u034f'+'\u0300'*n,
                      '👩'+'\u0301'*n+'\u200d👩', '🇸'*n,
                      '\u0915'+('\u094d\u093c'*n)+'\u0915'])
    for parts in records(ucd/'NormalizationTest.txt'):
        cols = [decode_hex(s) for s in parts[:5]]
        cases.extend(cols)
        text = cols[rng.randrange(5)]
        index = rng.randrange(len(text)+1)
        cases.append(text[:index] + rng.choice(['\u034f', '\u200d', '\u00ad']) + text[index:])
    for parts in records(ucd/'auxiliary/GraphemeBreakTest.txt'):
        cases.append(decode_hex(parts[0].replace('÷', '').replace('×', '')))
    # Mutate canonical compositions and compatibility/case mappings across boundaries.
    pool = [ord(x) for x in 'AaEeIİıßΣςé\x00 \n\r\t\u00a8\u034f\u200d\u0315\u0300\u0301\u0600\u0915\u094d\u093c👩🇸']
    for parts in records(ucd/'UnicodeData.txt'):
        if parts[5] and not parts[5].startswith('<'):
            text = decode_hex(parts[5])
            cases.extend([text[:1]+'\u034f'+text[1:], text+'\u0315\u0300'])
        if int(parts[3]) != 0:
            pool.append(int(parts[0],16))
    for parts in records(ucd/'DerivedNormalizationProps.txt'):
        if parts[1] == 'NFKC_CF':
            bounds = parts[0].split('..')
            for cp in sorted({int(bounds[0],16),int(bounds[-1],16)}):
                if 0xd800<=cp<=0xdfff:
                    continue
                pool.append(cp)
                cases.append('A'+chr(cp)+'\u0315\u0300')
    for _ in range(25000):
        chars = []
        for _ in range(rng.randrange(33)):
            cp = rng.choice(pool) if rng.randrange(4) else rng.randrange(0x110000)
            if not 0xd800<=cp<=0xdfff:
                chars.append(chr(cp))
        cases.append(''.join(chars))
    return cases

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--probe', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    icu = ICU()
    scalar_path = args.output/'all-scalars.bin'
    with scalar_path.open('wb') as out:
        subprocess.run([str(args.probe), '--all-scalars'], stdout=out, check=True)
    count = 0
    with scalar_path.open('rb') as source:
        for cp in range(0x110000):
            if 0xd800<=cp<=0xdfff:
                continue
            header = source.read(8)
            if len(header)!=8:
                raise RuntimeError('truncated scalar stream')
            actual_cp, size = struct.unpack('<II', header)
            actual = source.read(size).decode('utf-8')
            expected = icu.norm('NFKCCasefold', chr(cp))
            if actual_cp!=cp or actual!=expected:
                raise RuntimeError('ICU scalar mismatch: ' + hex(cp))
            count += 1
        if source.read(1):
            raise RuntimeError('extra scalar records')
    print('ICU76 Unicode16 all-scalar NFKC_CF: '+str(count)+' passed', flush=True)
    cases = corpus(ROOT/'testdata/ucd')
    input_path = args.output/'cross-scalar-inputs.jsonl'
    output_path = args.output/'cross-scalar-candidate.jsonl'
    with input_path.open('w', encoding='utf-8', newline='\n') as out:
        for text in cases:
            out.write(json.dumps({'input':text}, ensure_ascii=True, separators=(',',':'))+'\n')
    with input_path.open('rb') as inp, output_path.open('wb') as out:
        subprocess.run([str(args.probe)], stdin=inp, stdout=out, check=True)
    with output_path.open(encoding='utf-8') as output:
        for index, text in enumerate(cases):
            actual = json.loads(next(output))
            display = icu.display(text)
            public_key = icu.norm('NFKCCasefold', display)
            key = icu.norm('NFKCCasefold', icu.display(display))
            error = 'canonical16: empty display name' if not display else ('canonical16: empty canonical key' if not key else '')
            expected = {'nfc':icu.norm('NFC',text), 'fold':icu.norm('NFKCCasefold',text),
                        'display':display, 'public_key':public_key, 'name_display':'' if error else display,
                        'key':'' if error else key, 'error':error}
            if actual != expected:
                failure = {'index':index,'input':text,'actual':actual,'expected':expected}
                (args.output/'mismatch.json').write_text(json.dumps(failure,ensure_ascii=True,indent=2)+'\n')
                raise RuntimeError('ICU cross-scalar mismatch at '+str(index))
        if output.read():
            raise RuntimeError('extra probe output')
    result = {'status':'passed','swift_executed':False,'icu_version':icu.icu_version,
              'unicode_version':icu.unicode_version,'display_reference':'ICU root EGC + first-scalar White_Space; not Swift oracle',
              'all_scalar_count':count,'cross_scalar_cases':len(cases),'seed':SEED,
              'checks_per_cross_scalar':['NFC','NFKC_CF','display','public canonicalKey','initializer display','initializer key','name errors'],
              'python_version':platform.python_version(),'probe_sha256':digest(args.probe),
              'script_sha256':digest(Path(__file__)), 'source_manifest_sha256':digest(ROOT/'testdata/ucd/SOURCE.json'),
              'all_scalars_sha256':digest(scalar_path),'cross_scalar_inputs_sha256':digest(input_path),
              'cross_scalar_candidate_sha256':digest(output_path)}
    (args.output/'RESULT.json').write_text(json.dumps(result,indent=2)+'\n')
    print(json.dumps(result,indent=2))

if __name__=='__main__':
    main()
