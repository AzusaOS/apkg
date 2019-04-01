<?php

// convert .squashfs → .tpkg files by adding header
$sqfs = $_SERVER['argv'][1];
if (!$sqfs) die("please provide filename\n");
$fp = fopen($sqfs, 'rb');
if (!$fp) die("failed to open\n");

echo 'Preparing '.$sqfs."\n";

// read superblock info
// see: https://dr-emann.github.io/squashfs/
$info = unpack('Lmagic/Linocnt/Lmodtime/Lblksize/Lfragcnt/Scompression/Sblock_log/Sflags', fread($fp, 26));
if ($info['magic'] != 0x73717368) die("invalid squashfs tool, is it native endian?\n");

//var_dump($info);
switch($info['compression']) {
	case 1: // GZip
		break;
	default:
		die("unsupported compression ".$info['compression'].", please rebuild file\n");
}

if ($info['blksize'] != (1 << $info['block_log']))
	die("corrupted archive, block_log invalid\n");

$reserve_ino = $info['inocnt']; // number of inodes we need to reserve
$blksize = $info['blksize'];

// compute hash table
rewind($fp);
$table = '';
$blocks = 0;
while(!feof($fp)) {
	$table .= hash('sha256', fread($fp, $blksize), true);
	$blocks++;
}

// compute table hash
$table_hash = hash('sha256', $table, true);

echo 'table len  = '.strlen($table)." bytes ($blocks blocks)\n";
echo 'table hash = '.bin2hex($table_hash)."\n";

// grab filename
$name = explode('.', basename($sqfs, '.squashfs'));
$arch = array_pop($name);
$os = array_pop($name);
$names = [];
$tmp = [];
foreach($name as $frag) {
	$tmp[] = $frag;
	if (count($tmp) < 2) continue;
	$names[] = implode('.', $tmp);
}
array_shift($name);
array_shift($name);
$version = implode('.', $name);

$date = explode(' ', microtime());
$date = [(int)$date[1], (int)($date[0]*1000000000)];

// build metadata
$metadata = [
	'full_name' => basename($sqfs, '.squashfs'),
	'name' => $names[0],
	'version' => $version,
	'names' => $names,
	'os' => $os,
	'arch' => $arch,
	'size' => filesize($sqfs),
	'hash' => bin2hex($table_hash),
	'blocks' => $blocks,
	'block_size' => $blksize,
	'inodes' => $reserve_ino,
	'created' => $date,
];
$metadata = json_encode($metadata);
$metadata_len = strlen($metadata);

define('HEADER_LEN', 120);

// We use ECDSA signature
// signature is typically 72 bytes (ASN.1 r,s), and adding key id, ~104 bytes
// just in case we allocate 128 bytes for now...

$sign_offset = HEADER_LEN+$metadata_len + strlen($table);
$padding = 512-($sign_offset % 512);
if ($padding < 128) $padding += 512; // add extra padding if not enough for signature
$data_offset = $sign_offset + $padding;

echo "signature at $sign_offset, data at $data_offset\n";

// generate header (HEADER_LEN bytes)
$header = 'TPKG';
$header .= pack('N', 1); // 32bits, file format version
$header .= pack('J', 0); // 64bits, flags
$header .= pack('JJ', $date[0], $date[1]); // creation date
$header .= pack('N', HEADER_LEN); // MetaData offset int32
$header .= pack('N', $metadata_len); // metadata len
$header .= hash('sha256', $metadata, true); // metadata hash
$header .= pack('N', HEADER_LEN+$metadata_len); // Hash descriptor offset
$header .= pack('N', strlen($table)); // Hash descriptor length
$header .= $table_hash; // Hash descriptor hash
$header .= pack('N', $sign_offset);
$header .= pack('N', $data_offset);
if (strlen($header) != HEADER_LEN) die("invalid header len\n");

echo "header hash: ".hash('sha256', $header, false)."\n";

$base_name = $names[0];
$base_path = 'dist/data/'.str_replace('.','/',$base_name).'/'.basename($sqfs, '.squashfs').'.tpkg';
mkdir(dirname($base_path), 0755, true);

// generate output
$out = $base_path;
$outfp = fopen($out, 'wb');
fwrite($outfp, $header);
fwrite($outfp, $metadata);
fwrite($outfp, $table);
fwrite($outfp, str_repeat("\0", $padding));
rewind($fp);
stream_copy_to_stream($fp, $outfp);
fclose($outfp);

echo "Wrote $out\n";

