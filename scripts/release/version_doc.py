import yaml


def table(list_of_dicts):
    markdown_table = ""
    markdown_header = '| ' + ' | '.join(map(str, list_of_dicts[0].keys())) + ' |'
    markdown_header_separator = '|-----' * len(list_of_dicts[0].keys()) + '|'
    markdown_table += markdown_header + '\n'
    markdown_table += markdown_header_separator + '\n'
    for row in list_of_dicts:
        markdown_row = ""
        for key, col in row.items():
            markdown_row += '| ' + str(col) + ' '
        markdown_row += '|' + '\n'
        markdown_table += markdown_row
    return markdown_table


def version_doc(args):
    input_file = args.input_file
    out_file = args.out_file
    with open(input_file, 'r') as f:
        versions = yaml.safe_load(f)

    with open('template/version_doc.md', 'r') as vd:
        final_md = vd.read()

    table_md = table(versions)

    final_md = final_md.replace('<<TABLE>>', table_md)
    final_md = '<!--THIS DOC IS AUTO GENERATED-->\n' + final_md

    with open(out_file, 'w') as f:
        f.write(final_md)
